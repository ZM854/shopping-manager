package media

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestReserveValidationBeforeStorage(t *testing.T) {
	base := ReserveRequest{Purpose: Cover, ByteSize: 15 << 20, SHA256: strings.Repeat("a", 64)}
	for _, tc := range []struct {
		field  string
		key    string
		change func(*ReserveRequest)
	}{
		{"idempotencyKey", "short", func(*ReserveRequest) {}},
		{"purpose", "media-reserve-key-0001", func(r *ReserveRequest) { r.Purpose = "avatar" }},
		{"byteSize", "media-reserve-key-0001", func(r *ReserveRequest) { r.ByteSize = 0 }},
		{"byteSize", "media-reserve-key-0001", func(r *ReserveRequest) { r.ByteSize++ }},
		{"sha256", "media-reserve-key-0001", func(r *ReserveRequest) { r.SHA256 = strings.Repeat("A", 64) }},
	} {
		r := base
		tc.change(&r)
		got, err := NewService(nil).Reserve(context.Background(), 1, tc.key, r)
		var v *ValidationError
		if !errors.As(err, &v) || v.Field != tc.field || got.ID != 0 {
			t.Fatal("Invalid request reached storage", tc.field, err)
		}
	}
}

func TestOutputMetadataLimitsAndIndependentCopy(t *testing.T) {
	duration := int64(60000)
	good := []Variant{
		{Name: "video", ContentType: "video/mp4", ByteSize: 20 << 20, Width: 720, Height: 1280, DurationMilliseconds: &duration},
		{Name: "poster", ContentType: "image/jpeg", ByteSize: 200 << 10, Width: 360, Height: 640},
	}
	f := File{Purpose: StepVideo, Objects: []Object{{Name: "video", Key: "internal/video.mp4"}, {Name: "poster", Key: "internal/poster.jpg"}}}
	objects, total, err := completedObjects(f, good)
	if err != nil || total != outputBudget(StepVideo) {
		t.Fatal("Boundary variants rejected", err)
	}
	*objects[0].Metadata.DurationMilliseconds = 1
	objects[1].Metadata.Width = 1
	if duration != 60000 || good[1].Width != 360 {
		t.Fatal("Caller metadata was mutated")
	}
	for _, change := range []func(*Variant){
		func(v *Variant) { v.Width = 721 }, func(v *Variant) { v.ByteSize++ },
		func(v *Variant) { v.DurationMilliseconds = nil }, func(v *Variant) { v.ContentType = "video/webm" },
	} {
		v := good[0]
		change(&v)
		if validVariant(v) {
			t.Fatal("Invalid normalized variant accepted")
		}
	}
	if _, _, err := completedObjects(f, []Variant{good[1], good[0]}); err == nil {
		t.Fatal("Wrong variant order accepted")
	}
	if _, _, err := completedObjects(f, good[:1]); err == nil {
		t.Fatal("Incomplete video accepted")
	}
	encoded, _ := json.Marshal(objects)
	if strings.Contains(string(encoded), "internal/") {
		t.Fatal("Object keys leaked into JSON")
	}
}

func TestMetadataErrorsAreSafeAndUnwrap(t *testing.T) {
	cause := errors.New("SQL contains private file name")
	err := safeMetadataError(cause)
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "private") {
		t.Fatal("Storage error lost cause or leaked details")
	}
	if safeMetadataError(ErrMediaQuota) != ErrMediaQuota {
		t.Fatal("Domain error was hidden")
	}
	if _, err := NewService(nil).Fail(context.Background(), 1, 1, "raw stderr private filename"); err == nil {
		t.Fatal("Unsafe failure code accepted")
	}
}
