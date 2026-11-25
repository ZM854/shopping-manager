package recipe

import (
	"context"
	"errors"
	"strconv"

	"github.com/ZM854/shopping-manager/backend/internal/media"
)

// Public metadata only. Both recipe reads reuse this projection in their SQL
// snapshot. Storage keys and upload details never enter recipe DTOs.
const recipeMediaJSON = `jsonb_build_object(
	'id', m.id::text, 'purpose', m.purpose, 'variants', (
		SELECT jsonb_agg(jsonb_build_object(
			'name', v.name, 'contentType', v.content_type, 'byteSize', v.byte_size,
			'width', v.width, 'height', v.height, 'durationMilliseconds', v.duration_milliseconds
		) ORDER BY CASE v.name WHEN 'poster' THEN 2 ELSE 1 END)
		FROM recipe_media_variants v WHERE v.media_id = m.id
	))`

const recipeCoverJSON = `(SELECT ` + recipeMediaJSON + `
	FROM recipe_media m
	WHERE m.recipe_id = r.id AND m.owner_id = r.owner_id AND m.state = 'attached' AND m.purpose = 'cover')`

type mediaTarget struct {
	id           int64
	stepPosition int32 // zero for the cover
	position     int32
	field        string
}

func mediaTargets(req SaveRequest) ([]mediaTarget, error) {
	var targets []mediaTarget
	if req.CoverMediaID != nil {
		id, err := ParseID(*req.CoverMediaID)
		if err != nil {
			return nil, invalid("coverMediaId")
		}
		targets = append(targets, mediaTarget{id: id, field: "coverMediaId"})
	}
	for i, step := range req.Steps {
		for j, value := range step.MediaIDs {
			field := "steps." + strconv.Itoa(i) + ".mediaIds"
			id, err := ParseID(value)
			if err != nil {
				return nil, invalid(field)
			}
			targets = append(targets, mediaTarget{id: id, stepPosition: int32(i + 1), position: int32(j + 1), field: field})
		}
	}
	return targets, nil
}

type lockedMedia struct {
	purpose  media.Purpose
	state    media.State
	recipeID *int64
	bytes    int64
	expired  bool
}

func (r *transactionRepository) lockRecipeMedia(ctx context.Context, owner, recipeID int64, ids []int64) (map[int64]lockedMedia, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, purpose, state, recipe_id, accounted_bytes, COALESCE(expires_at <= statement_timestamp(), false)
		FROM recipe_media
		WHERE owner_id = $1 AND (recipe_id = $2 OR id = ANY($3::bigint[]))
		ORDER BY id FOR UPDATE`, owner, recipeID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make(map[int64]lockedMedia)
	for rows.Next() {
		var id int64
		var f lockedMedia
		if err := rows.Scan(&id, &f.purpose, &f.state, &f.recipeID, &f.bytes, &f.expired); err != nil {
			return nil, err
		}
		files[id] = f
	}
	return files, rows.Err()
}

func (r *transactionRepository) prepareRecipeMedia(ctx context.Context, owner, recipeID int64, targets []mediaTarget) error {
	ids := make([]int64, len(targets))
	for i, target := range targets {
		ids[i] = target.id
	}
	files, err := r.lockRecipeMedia(ctx, owner, recipeID, ids)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if _, ok := files[target.id]; !ok {
			return ErrNotFound
		}
	}
	var total int64
	videos := 0
	stepVideos := make(map[int32]bool)
	for _, target := range targets {
		f := files[target.id]
		if (target.stepPosition == 0) != (f.purpose == media.Cover) {
			return invalid(target.field)
		}
		switch {
		case f.state == media.DeletePending:
			return media.ErrMediaUnavailable
		case f.expired:
			return media.ErrUploadExpired
		case f.state == media.Attached && (f.recipeID == nil || *f.recipeID != recipeID):
			return media.ErrAlreadyAttached
		case f.state != media.Ready && f.state != media.Attached:
			return media.ErrMediaNotReady
		}
		if f.purpose == media.StepVideo {
			videos++
			if stepVideos[target.stepPosition] || videos > maxRecipeVideo {
				return invalid(target.field)
			}
			stepVideos[target.stepPosition] = true
		}
		total += f.bytes // Ready/attached charges are the sum of stored variants.
	}
	if total > maxRecipeBytes {
		return media.ErrMediaQuota
	}
	// Clear old positions before writing new ones. Retained files pass through
	// ready inside the same transaction, so immediate unique indexes allow
	// swaps, and deleting an old step cannot mark a moved file for deletion.
	_, err = r.db.Exec(ctx, `
		UPDATE recipe_media
		SET state = CASE WHEN id = ANY($3::bigint[]) THEN 'ready' ELSE 'delete_pending' END,
			recipe_id = NULL, step_id = NULL, position = NULL,
			expires_at = created_at + interval '24 hours', updated_at = clock_timestamp()
		WHERE owner_id = $1 AND recipe_id = $2 AND state = 'attached'`, owner, recipeID, ids)
	return err
}

func (r *transactionRepository) attachRecipeMedia(ctx context.Context, owner, recipeID int64, targets []mediaTarget) error {
	if len(targets) == 0 {
		return nil
	}
	ids := make([]int64, len(targets))
	steps, positions := make([]int32, len(targets)), make([]int32, len(targets))
	for i, target := range targets {
		ids[i], steps[i], positions[i] = target.id, target.stepPosition, target.position
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE recipe_media m SET state = 'attached', recipe_id = $2, step_id = s.id,
			position = NULLIF(target.media_position, 0), expires_at = NULL,
			upload_slot_reserved = false, updated_at = clock_timestamp()
		FROM unnest($3::bigint[], $4::integer[], $5::integer[]) AS target(id, step_position, media_position)
		LEFT JOIN recipe_steps s ON s.recipe_id = $2 AND s.position = target.step_position
		WHERE m.owner_id = $1 AND m.id = target.id AND m.state = 'ready'
			AND (target.step_position = 0 OR s.id IS NOT NULL)`, owner, recipeID, ids, steps, positions)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(targets)) {
		return errors.New("recipe media binding incomplete")
	}
	return nil
}
