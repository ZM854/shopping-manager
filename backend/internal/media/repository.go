package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transaction interface {
	Now(context.Context) (time.Time, error)
	LockOwner(context.Context, int64) error
	Usage(context.Context, int64) (Usage, error)
	GetFile(context.Context, int64, int64) (File, error)
	CreateFile(context.Context, int64, ReserveRequest) (File, error)
	SaveFile(context.Context, File) error
	DeleteFile(context.Context, int64, int64) error
	Claim(context.Context, int64, string, string) (Replay, error)
	SaveReplay(context.Context, int64, string, Reservation, []byte) error
}

type Store interface {
	InTx(context.Context, func(Transaction) error) error
}
type Repository struct{ pool *pgxpool.Pool }
type transaction struct{ db pgx.Tx }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) InTx(ctx context.Context, fn func(Transaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := fn(&transaction{db: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (t *transaction) LockOwner(ctx context.Context, owner int64) error {
	var id int64
	return missing(t.db.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&id))
}

func (t *transaction) Usage(ctx context.Context, owner int64) (Usage, error) {
	var u Usage

	err := t.db.QueryRow(ctx, `SELECT COALESCE(sum(accounted_bytes),0),
		COALESCE(sum(source_reserved_bytes),0), count(*) FILTER (WHERE upload_slot_reserved)
		FROM recipe_media WHERE owner_id=$1`, owner).Scan(&u.AccountedBytes, &u.SourceBytes, &u.UploadSlots)
	
	return u, err
}

func (t *transaction) GetFile(ctx context.Context, owner, id int64) (File, error) {
	var f File
	err := t.db.QueryRow(ctx, `SELECT id,owner_id,purpose,byte_size,sha256,state,
		created_at,updated_at,expires_at,error_code,accounted_bytes,source_reserved_bytes,
		upload_slot_reserved,recipe_id,step_id,position
		FROM recipe_media WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id).Scan(
		&f.ID, &f.OwnerID, &f.Purpose, &f.ByteSize, &f.SHA256, &f.State, &f.CreatedAt, &f.UpdatedAt,
		&f.ExpiresAt, &f.ErrorCode, &f.AccountedBytes, &f.SourceReservedBytes,
		&f.UploadSlotReserved, &f.RecipeID, &f.StepID, &f.Position)
	if err != nil {
		return File{}, missing(err)
	}
	f.CreatedAt, f.UpdatedAt = f.CreatedAt.UTC(), f.UpdatedAt.UTC()
	if f.ExpiresAt != nil {
		v := f.ExpiresAt.UTC()
		f.ExpiresAt = &v
	}
	rows, err := t.db.Query(ctx, `SELECT name,object_key,content_type,byte_size,width,height,duration_milliseconds
		FROM recipe_media_variants WHERE media_id=$1 ORDER BY CASE name WHEN 'poster' THEN 2 ELSE 1 END`, id)
	if err != nil {
		return File{}, err
	}
	defer rows.Close()
	f.Objects = make([]Object, 0, 2)
	for rows.Next() {
		var o Object
		var contentType *string
		var size *int64
		var width, height *int
		var duration *int64
		if err := rows.Scan(&o.Name, &o.Key, &contentType, &size, &width, &height, &duration); err != nil {
			return File{}, err
		}
		if contentType != nil {
			o.Metadata = &Variant{Name: o.Name, ContentType: *contentType, ByteSize: *size, Width: *width, Height: *height, DurationMilliseconds: duration}
		}
		f.Objects = append(f.Objects, o)
	}
	if err := rows.Err(); err != nil {
		return File{}, err
	}
	return f, nil
}

func (t *transaction) CreateFile(ctx context.Context, owner int64, req ReserveRequest) (File, error) {
	var id int64
	err := t.db.QueryRow(ctx, `INSERT INTO recipe_media
		(owner_id,purpose,byte_size,sha256,created_at,expires_at,accounted_bytes,source_reserved_bytes)
		SELECT $1,$2,$3,$4,at,at+interval '24 hours',$5,$3 FROM (SELECT clock_timestamp() AS at) n RETURNING id`,
		owner, req.Purpose, req.ByteSize, req.SHA256, outputBudget(req.Purpose)).Scan(&id)
	if err != nil {
		return File{}, err
	}
	for _, name := range variantNames(req.Purpose) {
		ext := ".jpg"
		if name == "video" {
			ext = ".mp4"
		}
		key := fmt.Sprintf("recipe-media/%d/%s%s", id, name, ext)
		tag, err := t.db.Exec(ctx, `INSERT INTO recipe_media_variants (media_id,purpose,name,object_key)
			VALUES ($1,$2,$3,$4)`, id, req.Purpose, name, key)
		if err != nil {
			return File{}, err
		}
		if tag.RowsAffected() != 1 {
			return File{}, ErrNotFound
		}
	}
	return t.GetFile(ctx, owner, id)
}

func (t *transaction) SaveFile(ctx context.Context, f File) error {
	tag, err := t.db.Exec(ctx, `UPDATE recipe_media SET state=$3,error_code=$4,accounted_bytes=$5,
		source_reserved_bytes=$6,upload_slot_reserved=$7,expires_at=$8,
		recipe_id=$9,step_id=$10,position=$11,updated_at=clock_timestamp()
		WHERE owner_id=$1 AND id=$2`, f.OwnerID, f.ID, f.State, f.ErrorCode, f.AccountedBytes,
		f.SourceReservedBytes, f.UploadSlotReserved, f.ExpiresAt, f.RecipeID, f.StepID, f.Position)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	for _, o := range f.Objects {
		if o.Metadata == nil {
			continue
		}
		v := o.Metadata
		tag, err = t.db.Exec(ctx, `UPDATE recipe_media_variants SET content_type=$3,byte_size=$4,
			width=$5,height=$6,duration_milliseconds=$7 WHERE media_id=$1 AND name=$2`,
			f.ID, o.Name, v.ContentType, v.ByteSize, v.Width, v.Height, v.DurationMilliseconds)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
	}
	return nil
}

func (t *transaction) DeleteFile(ctx context.Context, owner, id int64) error {
	tag, err := t.db.Exec(ctx, `DELETE FROM recipe_media WHERE owner_id=$1 AND id=$2
		AND state='delete_pending' AND source_reserved_bytes=0`, owner, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (t *transaction) Claim(ctx context.Context, owner int64, key, hash string) (Replay, error) {
	var r Replay
	err := t.db.QueryRow(ctx, `INSERT INTO recipe_media_idempotency AS old
		(owner_id,key,request_hash,expires_at) VALUES ($1,$2,$3,clock_timestamp()+interval '24 hours')
		ON CONFLICT(owner_id,key) DO UPDATE SET
		 request_hash=CASE WHEN old.expires_at<=statement_timestamp() THEN EXCLUDED.request_hash ELSE old.request_hash END,
		 resource_id=CASE WHEN old.expires_at<=statement_timestamp() THEN NULL ELSE old.resource_id END,
		 response=CASE WHEN old.expires_at<=statement_timestamp() THEN NULL ELSE old.response END,
		 expires_at=CASE WHEN old.expires_at<=statement_timestamp() THEN EXCLUDED.expires_at ELSE old.expires_at END
		RETURNING request_hash,resource_id,response`, owner, key, hash).Scan(&r.Hash, &r.ResourceID, &r.Response)
	return r, err
}

func (t *transaction) SaveReplay(ctx context.Context, owner int64, key string, r Reservation, body []byte) error {
	tag, err := t.db.Exec(ctx, `UPDATE recipe_media_idempotency SET resource_id=$3,response=$4::jsonb,
		expires_at=greatest(expires_at,clock_timestamp()+interval '24 hours') WHERE owner_id=$1 AND key=$2`, owner, key, r.ID, body)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (t *transaction) Now(ctx context.Context) (time.Time, error) {
	var now time.Time
	err := t.db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}

func (r *Repository) RecoverInterrupted(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT owner_id FROM recipe_media WHERE state='processing' AND owner_id IS NOT NULL ORDER BY owner_id`)
	if err != nil {
		return safeMetadataError(err)
	}
	var owners []int64
	for rows.Next() {
		var owner int64
		if err = rows.Scan(&owner); err != nil {
			rows.Close()
			return safeMetadataError(err)
		}
		owners = append(owners, owner)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return safeMetadataError(err)
	}
	for _, owner := range owners {
		err = r.InTx(ctx, func(tx Transaction) error {
			if err := tx.LockOwner(ctx, owner); err != nil {
				if errors.Is(err, ErrNotFound) {
					return nil
				}
				return err
			}
			t := tx.(*transaction)
			_, err := t.db.Exec(ctx, `WITH instant AS MATERIALIZED (SELECT clock_timestamp() AS at)
				UPDATE recipe_media SET
				state=CASE WHEN expires_at<=instant.at THEN 'delete_pending' ELSE 'failed' END,
				error_code=CASE WHEN expires_at<=instant.at THEN NULL ELSE 'media_processing_interrupted' END,
				source_reserved_bytes=0,updated_at=clock_timestamp()
				FROM instant WHERE owner_id=$1 AND state='processing'`, owner)
			return err
		})
		if err != nil {
			return safeMetadataError(err)
		}
	}
	_, err = r.pool.Exec(ctx, `UPDATE recipe_media SET source_reserved_bytes=0 WHERE owner_id IS NULL AND state='delete_pending'`)
	return safeMetadataError(err)
}
