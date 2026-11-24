//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/media"
	"github.com/jackc/pgx/v5/pgconn"
)

func mediaRequest(p media.Purpose, size int64) media.ReserveRequest {
	return media.ReserveRequest{Purpose: p, ByteSize: size, SHA256: strings.Repeat("a", 64)}
}
func reserveMedia(t *testing.T, s *media.Service, owner int64, key string, p media.Purpose, size int64) media.Reservation {
	t.Helper()
	r, err := s.Reserve(context.Background(), owner, key, mediaRequest(p, size))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func readyMedia(t *testing.T, s *media.Service, owner int64, r media.Reservation) media.File {
	t.Helper()
	ctx := context.Background()
	if _, err := s.BeginProcessing(ctx, owner, r.ID, r.ByteSize, r.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseSource(ctx, owner, r.ID); err != nil {
		t.Fatal(err)
	}
	var variants []media.Variant
	switch r.Purpose {
	case media.Cover:
		variants = []media.Variant{{Name: "cover", ContentType: "image/jpeg", ByteSize: 1234, Width: 640, Height: 360}}
	case media.StepImage:
		variants = []media.Variant{{Name: "image", ContentType: "image/jpeg", ByteSize: 2345, Width: 800, Height: 600}}
	case media.StepVideo:
		d := int64(2500)
		variants = []media.Variant{{Name: "video", ContentType: "video/mp4", ByteSize: 3456, Width: 1280, Height: 720, DurationMilliseconds: &d},
			{Name: "poster", ContentType: "image/jpeg", ByteSize: 4567, Width: 640, Height: 360}}
	}
	f, err := s.Complete(ctx, owner, r.ID, variants)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMediaMetadataLifecycleAndReplay(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := media.NewService(media.NewRepository(f.pool))
	ctx := context.Background()
	migrationExec(t, f.pool, `SELECT setval('recipe_media_id_seq',9007199254740992)`)
	r := reserveMedia(t, s, f.first, "media-lifecycle-0001", media.Cover, 15<<20)
	if r.ID != 9007199254740993 || r.ExpiresAt.Sub(r.CreatedAt) != media.UploadTTL || r.CreatedAt.Location() != time.UTC {
		t.Fatal("Reservation ID/time lost")
	}
	body, _ := json.Marshal(r)
	if strings.Contains(string(body), "recipe-media/") || !strings.Contains(string(body), `"id":"9007199254740993"`) {
		t.Fatal("Invalid public reservation")
	}
	for _, owner := range []int64{f.second} {
		if _, err := s.GetFile(ctx, owner, r.ID); !errors.Is(err, media.ErrNotFound) {
			t.Fatal("Foreign read", err)
		}
		if err := s.Cancel(ctx, owner, r.ID); !errors.Is(err, media.ErrNotFound) {
			t.Fatal("Foreign delete", err)
		}
		if _, err := s.BeginProcessing(ctx, owner, r.ID, r.ByteSize, r.SHA256); !errors.Is(err, media.ErrNotFound) {
			t.Fatal("Foreign processing", err)
		}
		if _, err := s.Complete(ctx, owner, r.ID, nil); !errors.Is(err, media.ErrNotFound) {
			t.Fatal("Foreign completion", err)
		}
		if err := s.ReleaseSource(ctx, owner, r.ID); !errors.Is(err, media.ErrNotFound) {
			t.Fatal("Foreign source release", err)
		}
	}
	if _, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize-1, r.SHA256); !errors.Is(err, media.ErrContentMismatch) {
		t.Fatal("Wrong bytes accepted", err)
	}
	if _, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize, r.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(ctx, f.first, r.ID); !errors.Is(err, media.ErrUploadInProgress) {
		t.Fatal("Processing cancelled", err)
	}
	if _, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize, r.SHA256); !errors.Is(err, media.ErrUploadInProgress) {
		t.Fatal("Parallel processing accepted", err)
	}
	if _, err := s.Fail(ctx, f.first, r.ID, "invalid_media"); !errors.Is(err, media.ErrUploadInProgress) {
		t.Fatal("Tmp budget released implicitly", err)
	}
	if err := s.ReleaseSource(ctx, f.first, r.ID); err != nil {
		t.Fatal(err)
	}
	failed, err := s.Fail(ctx, f.first, r.ID, "invalid_media")
	if err != nil || failed.State != media.Failed || failed.SourceReservedBytes != 0 || !failed.UploadSlotReserved {
		t.Fatal("Invalid failed state", err)
	}
	loaded, err := s.GetFile(ctx, f.first, r.ID)
	if err != nil || !loaded.ExpiresAt.Equal(r.ExpiresAt) {
		t.Fatal("Status prolonged TTL", err)
	}
	prepared, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize, r.SHA256)
	if err != nil || prepared.SourceReservedBytes != r.ByteSize {
		t.Fatal("Retry did not reserve source again", err)
	}
	if err := s.ReleaseSource(ctx, f.first, r.ID); err != nil {
		t.Fatal(err)
	}
	ready, err := s.Complete(ctx, f.first, r.ID, []media.Variant{{Name: "cover", ContentType: "image/jpeg", ByteSize: 1234, Width: 640, Height: 360}})
	if err != nil || ready.State != media.Ready || ready.AccountedBytes != 1234 || ready.Objects[0].Metadata == nil {
		t.Fatal("Completion failed", err)
	}
	replayed, err := s.Reserve(ctx, f.first, "media-lifecycle-0001", mediaRequest(media.Cover, 15<<20))
	if err != nil || !reflect.DeepEqual(replayed, r) {
		t.Fatal("Replay returned current status", err)
	}
	_, err = s.Reserve(ctx, f.first, "media-lifecycle-0001", mediaRequest(media.Cover, 1))
	if !errors.Is(err, media.ErrIdempotencyConflict) {
		t.Fatal("Body conflict lost", err)
	}
	if got, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize, r.SHA256); err != nil || got.State != media.Ready || got.AccountedBytes != 1234 {
		t.Fatal("Ready replay reprocessed", err)
	}
	if _, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize, strings.Repeat("b", 64)); !errors.Is(err, media.ErrContentMismatch) {
		t.Fatal("Ready content mismatch lost", err)
	}
	if err := s.Cancel(ctx, f.first, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(ctx, f.first, r.ID); err != nil {
		t.Fatal("Cancel not idempotent", err)
	}
	pending, err := s.GetFile(ctx, f.first, r.ID)
	if err != nil || pending.AccountedBytes != 1234 || pending.Objects[0].Key != ready.Objects[0].Key || !pending.UploadSlotReserved {
		t.Fatal("Cancel lost quota or key", err)
	}
	if _, err := s.Reserve(ctx, f.first, "media-lifecycle-0001", mediaRequest(media.Cover, 15<<20)); !errors.Is(err, media.ErrResourceDeleted) {
		t.Fatal("Cancelled replay recreated", err)
	}
	if err := s.ConfirmDeletion(ctx, f.first, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(ctx, f.first, "media-lifecycle-0001", mediaRequest(media.Cover, 15<<20)); !errors.Is(err, media.ErrResourceDeleted) {
		t.Fatal("Deleted replay recreated", err)
	}
	migrationExec(t, f.pool, `UPDATE recipe_media_idempotency SET expires_at=clock_timestamp()-interval '1 second' WHERE owner_id=$1`, f.first)
	fresh := reserveMedia(t, s, f.first, "media-lifecycle-0001", media.Cover, 1)
	if fresh.ID == r.ID {
		t.Fatal("Expired key did not permit a new result")
	}
	other := reserveMedia(t, s, f.second, "media-lifecycle-0001", media.Cover, 1)
	if other.ID == fresh.ID {
		t.Fatal("Owner scopes mixed")
	}
}

func TestMediaMetadataConcurrentQuotasAndClaims(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := media.NewService(media.NewRepository(f.pool))
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Reserve(ctx, f.first, fmt.Sprintf("media-concurrent-%08d", i), mediaRequest(media.StepVideo, 50<<20))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, media.ErrUploadQuota) {
			t.Fatal(err)
		}
	}
	if success != 4 || migrationScalar(t, f.pool, `SELECT sum(source_reserved_bytes)::text FROM recipe_media WHERE owner_id=$1`, f.first) != "209715200" {
		t.Fatal("Concurrent source quota exceeded", success)
	}
	ids := make(chan int64, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Reserve(ctx, f.second, "media-same-key-0001", mediaRequest(media.StepImage, 1))
			ids <- r.ID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	var first int64
	for id := range ids {
		if first == 0 {
			first = id
		}
		if id != first || id <= 0 {
			t.Fatal("Concurrent claim created duplicates")
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM recipe_media WHERE owner_id=$1`, f.second) != "1" {
		t.Fatal("Duplicate file rows")
	}
}

func TestMediaMetadataSlotsAndPendingAccounting(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := media.NewService(media.NewRepository(f.pool))
	ctx := context.Background()
	var first media.Reservation
	for i := 0; i < 25; i++ {
		r := reserveMedia(t, s, f.first, fmt.Sprintf("media-slots-%08d", i), media.Cover, 1)
		if i == 0 {
			first = r
		}
	}
	_, err := s.Reserve(ctx, f.first, "media-over-slots-0001", mediaRequest(media.Cover, 1))
	if !errors.Is(err, media.ErrUploadQuota) {
		t.Fatal("Slot limit ignored", err)
	}
	if err := s.Cancel(ctx, f.first, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmDeletion(ctx, f.first, first.ID); !errors.Is(err, media.ErrUploadInProgress) {
		t.Fatal("Source charge lost", err)
	}
	_, err = s.Reserve(ctx, f.first, "media-over-slots-0001", mediaRequest(media.Cover, 1))
	if !errors.Is(err, media.ErrUploadQuota) {
		t.Fatal("Pending slot released early", err)
	}
	if err := s.ReleaseSource(ctx, f.first, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmDeletion(ctx, f.first, first.ID); err != nil {
		t.Fatal(err)
	}
	reserveMedia(t, s, f.first, "media-over-slots-0001", media.Cover, 1)
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM recipe_media_idempotency WHERE owner_id=$1`, f.first) != "26" {
		t.Fatal("Failed claims were not rolled back or old replay was removed")
	}
}

func TestMediaMetadataRollbackAndExpiry(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := media.NewService(media.NewRepository(f.pool))
	ctx := context.Background()
	migrationExec(t, f.pool, `CREATE FUNCTION reject_media_replay() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private SQL filename'; END $$;
		CREATE TRIGGER reject_media_replay BEFORE UPDATE OF response ON recipe_media_idempotency FOR EACH ROW EXECUTE FUNCTION reject_media_replay()`)
	r, err := s.Reserve(ctx, f.first, "media-rollback-0001", mediaRequest(media.StepVideo, 100))
	var pgerr *pgconn.PgError
	if err == nil || r.ID != 0 || strings.Contains(err.Error(), "private") || !errors.As(err, &pgerr) {
		t.Fatal("Unsafe/non-atomic reservation error", err)
	}
	if migrationScalar(t, f.pool, `SELECT (SELECT count(*) FROM recipe_media)+(SELECT count(*) FROM recipe_media_variants)+(SELECT count(*) FROM recipe_media_idempotency)`) != "0" {
		t.Fatal("Partial reservation survived rollback")
	}
	migrationExec(t, f.pool, `DROP TRIGGER reject_media_replay ON recipe_media_idempotency`)
	migrationExec(t, f.pool, `CREATE OR REPLACE FUNCTION reject_media_replay() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$;
		CREATE TRIGGER reject_media_replay BEFORE UPDATE OF response ON recipe_media_idempotency FOR EACH ROW EXECUTE FUNCTION reject_media_replay()`)
	r, err = s.Reserve(ctx, f.first, "media-rollback-0001", mediaRequest(media.StepVideo, 100))
	if !errors.Is(err, media.ErrNotFound) || r.ID != 0 {
		t.Fatal("Suppressed replay write returned success", err)
	}
	migrationExec(t, f.pool, `DROP TRIGGER reject_media_replay ON recipe_media_idempotency;
		CREATE TRIGGER reject_media_variant BEFORE INSERT ON recipe_media_variants FOR EACH ROW EXECUTE FUNCTION reject_media_replay()`)
	r, err = s.Reserve(ctx, f.first, "media-rollback-0001", mediaRequest(media.StepVideo, 100))
	if !errors.Is(err, media.ErrNotFound) || r.ID != 0 {
		t.Fatal("Missing planned key returned success", err)
	}
	if migrationScalar(t, f.pool, `SELECT (SELECT count(*) FROM recipe_media)+(SELECT count(*) FROM recipe_media_idempotency)`) != "0" {
		t.Fatal("Suppressed write left a partial reservation")
	}
	migrationExec(t, f.pool, `DROP TRIGGER reject_media_variant ON recipe_media_variants`)
	r = reserveMedia(t, s, f.first, "media-rollback-0001", media.StepVideo, 100)
	if _, err := s.BeginProcessing(ctx, f.first, r.ID, r.ByteSize, r.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseSource(ctx, f.first, r.ID); err != nil {
		t.Fatal(err)
	}
	d := int64(1000)
	variants := []media.Variant{{Name: "video", ContentType: "video/mp4", ByteSize: 1000, Width: 1280, Height: 720, DurationMilliseconds: &d},
		{Name: "poster", ContentType: "image/jpeg", ByteSize: 100, Width: 640, Height: 360}}
	migrationExec(t, f.pool, `CREATE FUNCTION reject_media_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private commit'; END $$;
		CREATE CONSTRAINT TRIGGER reject_media_ready AFTER UPDATE ON recipe_media DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW WHEN (NEW.state='ready') EXECUTE FUNCTION reject_media_ready()`)
	got, err := s.Complete(ctx, f.first, r.ID, variants)
	if err == nil || got.ID != 0 || strings.Contains(err.Error(), "private") {
		t.Fatal("Commit failure returned success", err)
	}
	current, err := s.GetFile(ctx, f.first, r.ID)
	if err != nil || current.State != media.Processing || current.Objects[0].Metadata != nil || current.Objects[1].Metadata != nil {
		t.Fatal("Completion partially committed", err)
	}
	migrationExec(t, f.pool, `DROP TRIGGER reject_media_ready ON recipe_media;
		UPDATE recipe_media SET created_at=created_at-interval '2 days',expires_at=clock_timestamp()-interval '1 second'`)
	if _, err := s.Reserve(ctx, f.first, "media-rollback-0001", mediaRequest(media.StepVideo, 100)); !errors.Is(err, media.ErrUploadExpired) {
		t.Fatal("Expired reservation replay accepted", err)
	}
	if got, err := s.Complete(ctx, f.first, r.ID, variants); !errors.Is(err, media.ErrUploadExpired) || got.ID != 0 {
		t.Fatal("Expired processing became ready", err)
	}
	pending, err := s.GetFile(ctx, f.first, r.ID)
	if err != nil || pending.State != media.DeletePending || pending.AccountedBytes != (20<<20)+(200<<10) || len(pending.Objects) != 2 {
		t.Fatal("Expired output keys/budget lost", err)
	}
}

func TestMediaMetadataConcurrentOutputBudget(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := media.NewService(media.NewRepository(f.pool))
	ctx := context.Background()
	// Charge pending former attachments up to exactly one cover below 1 GiB.
	// They have no upload slots/source bytes, but still occupy physical S3 space.
	migrationExec(t, f.pool, `INSERT INTO recipe_media (owner_id,purpose,byte_size,sha256,state,expires_at,
		accounted_bytes,source_reserved_bytes,upload_slot_reserved)
		SELECT $1,'step-video',1,repeat('a',64),'delete_pending',clock_timestamp()+interval '1 day',
		least(21176320,1073741824-204800-(n-1)*21176320),0,false FROM generate_series(1,51) n`, f.first)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Reserve(ctx, f.first, fmt.Sprintf("media-output-race-%08d", i), mediaRequest(media.Cover, 1))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, media.ErrMediaQuota) {
			t.Fatal(err)
		}
	}
	if success != 1 || migrationScalar(t, f.pool, `SELECT sum(accounted_bytes)::text FROM recipe_media WHERE owner_id=$1`, f.first) != "1073741824" {
		t.Fatal("Concurrent normalized quota exceeded", success)
	}
	id, err := strconv.ParseInt(migrationScalar(t, f.pool, `SELECT min(id)::text FROM recipe_media WHERE state='delete_pending'`), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmDeletion(ctx, f.first, id); err != nil {
		t.Fatal(err)
	}
	reserveMedia(t, s, f.first, "media-output-after-delete", media.Cover, 1)
}

func attachMedia(t *testing.T, f *recipeRepositoryFixture, id, recipeID int64, stepID *int64, position *int) {
	t.Helper()
	migrationExec(t, f.pool, `UPDATE recipe_media SET state='attached',recipe_id=$2,step_id=$3,
		position=$4,expires_at=NULL,upload_slot_reserved=false WHERE id=$1`, id, recipeID, stepID, position)
}

func TestMediaMetadataBindingsAndCascadeKeepKeys(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := media.NewService(media.NewRepository(f.pool))
	ctx := context.Background()
	r := f.create(t, f.first, recipeRepositoryRequest("С медиа"))
	other := f.create(t, f.first, recipeRepositoryRequest("Другой"))
	foreign := f.create(t, f.second, recipeRepositoryRequest("Чужой"))
	cover := reserveMedia(t, s, f.first, "media-cascade-cover-0001", media.Cover, 1)
	readyMedia(t, s, f.first, cover)
	recipeMigrationReject(t, f.pool, "23503", `UPDATE recipe_media SET state='attached',recipe_id=$2,expires_at=NULL,upload_slot_reserved=false WHERE id=$1`, cover.ID, foreign.ID)
	attachMedia(t, f, cover.ID, r.ID, nil, nil)
	if err := s.Cancel(ctx, f.first, cover.ID); !errors.Is(err, media.ErrAlreadyAttached) {
		t.Fatal("Attached cover cancelled", err)
	}
	if replay, err := s.Reserve(ctx, f.first, "media-cascade-cover-0001", mediaRequest(media.Cover, 1)); err != nil || !reflect.DeepEqual(replay, cover) {
		t.Fatal("Attached reservation replay changed", err)
	}
	duplicate := reserveMedia(t, s, f.first, "media-cascade-cover-0002", media.Cover, 1)
	readyMedia(t, s, f.first, duplicate)
	recipeMigrationReject(t, f.pool, "23505", `UPDATE recipe_media SET state='attached',recipe_id=$2,expires_at=NULL,upload_slot_reserved=false WHERE id=$1`, duplicate.ID, r.ID)
	image := reserveMedia(t, s, f.first, "media-cascade-image-0001", media.StepImage, 1)
	ready := readyMedia(t, s, f.first, image)
	recipeMigrationReject(t, f.pool, "23503", `UPDATE recipe_media SET state='attached',recipe_id=$2,step_id=$3,position=1,expires_at=NULL,upload_slot_reserved=false WHERE id=$1`, image.ID, r.ID, other.Steps[0].ID)
	position := 1
	attachMedia(t, f, image.ID, r.ID, &r.Steps[0].ID, &position)
	video := reserveMedia(t, s, f.first, "media-cascade-video-0001", media.StepVideo, 1)
	readyMedia(t, s, f.first, video)
	position = 2
	attachMedia(t, f, video.ID, r.ID, &r.Steps[0].ID, &position)
	video2 := reserveMedia(t, s, f.first, "media-cascade-video-0002", media.StepVideo, 1)
	readyMedia(t, s, f.first, video2)
	recipeMigrationReject(t, f.pool, "23505", `UPDATE recipe_media SET state='attached',recipe_id=$2,step_id=$3,position=3,expires_at=NULL,upload_slot_reserved=false WHERE id=$1`, video2.ID, r.ID, r.Steps[0].ID)
	recipeMigrationReject(t, f.pool, "23514", `UPDATE recipe_media SET position=5 WHERE id=$1`, image.ID)
	key := ready.Objects[0].Key
	migrationExec(t, f.pool, `UPDATE recipe_steps SET position=position+10 WHERE id=$1`, r.Steps[0].ID)
	unchanged, err := s.GetFile(ctx, f.first, image.ID)
	if err != nil || unchanged.State != media.Attached || unchanged.StepID == nil || *unchanged.StepID != r.Steps[0].ID {
		t.Fatal("Position changed stable binding", err)
	}
	migrationExec(t, f.pool, `DELETE FROM recipe_steps WHERE id=$1`, r.Steps[0].ID)
	pending, err := s.GetFile(ctx, f.first, image.ID)
	if err != nil || pending.State != media.DeletePending || pending.RecipeID != nil || pending.StepID != nil ||
		pending.Objects[0].Key != key || pending.AccountedBytes != 2345 || pending.UploadSlotReserved || pending.ExpiresAt == nil {
		t.Fatal("Step cascade lost key/quota", err)
	}
	migrationExec(t, f.pool, `DELETE FROM recipes WHERE id=$1`, r.ID)
	pending, err = s.GetFile(ctx, f.first, cover.ID)
	if err != nil || pending.State != media.DeletePending || len(pending.Objects) != 1 {
		t.Fatal("Recipe cascade lost cover", err)
	}
	before := migrationScalar(t, f.pool, `SELECT string_agg(object_key,',' ORDER BY object_key) FROM recipe_media_variants`)
	migrationExec(t, f.pool, `DELETE FROM users WHERE id=$1`, f.first)
	if migrationScalar(t, f.pool, `SELECT string_agg(object_key,',' ORDER BY object_key) FROM recipe_media_variants`) != before ||
		migrationScalar(t, f.pool, `SELECT count(*)::text FROM recipe_media WHERE owner_id IS NOT NULL OR state<>'delete_pending'`) != "0" {
		t.Fatal("Owner cascade lost object keys or left accessible media")
	}
	if _, err := s.GetFile(ctx, f.first, image.ID); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("Orphan media still accessible", err)
	}
}
