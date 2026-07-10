package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestArtifact_Validate(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	base := func() domain.Artifact {
		return domain.Artifact{
			ID: "a1", OwnerID: "u1", NodeID: "n1", Slug: "bild", Name: "bild.png",
			Mime: "image/png", SizeBytes: 1024, Ref: "abc123def456",
			CreatedAt: now, UpdatedAt: now,
		}
	}

	// valid image
	if err := base().Validate(); err != nil {
		t.Fatalf("valid PNG image: %v", err)
	}

	// valid PDF download
	pdf := base()
	pdf.Mime = "application/pdf"
	pdf.Name = "doc.pdf"
	if err := pdf.Validate(); err != nil {
		t.Fatalf("valid PDF download: %v", err)
	}

	// SVG rejected
	svg := base()
	svg.Mime = "image/svg+xml"
	if err := svg.Validate(); !errors.Is(err, domain.ErrInvalidArtifact) {
		t.Errorf("svg mime should be rejected, got %v", err)
	}

	// oversize rejected
	big := base()
	big.SizeBytes = domain.MaxArtifactBytes + 1
	if err := big.Validate(); !errors.Is(err, domain.ErrInvalidArtifact) {
		t.Errorf("oversize should be rejected, got %v", err)
	}

	// empty slug rejected
	emptySlug := base()
	emptySlug.Slug = ""
	if err := emptySlug.Validate(); !errors.Is(err, domain.ErrInvalidArtifact) {
		t.Errorf("empty slug should be rejected, got %v", err)
	}

	// slash-segmented slug rejected (flat token only)
	slashSlug := base()
	slashSlug.Slug = "slug/mit/slash"
	if err := slashSlug.Validate(); !errors.Is(err, domain.ErrInvalidArtifact) {
		t.Errorf("slash slug should be rejected, got %v", err)
	}

	// empty name rejected
	emptyName := base()
	emptyName.Name = ""
	if err := emptyName.Validate(); !errors.Is(err, domain.ErrInvalidArtifact) {
		t.Errorf("empty name should be rejected, got %v", err)
	}
}

func TestArtifactSlugOK(t *testing.T) {
	ok := []string{"bild", "bild-1", "a1", "mein-bild-2026"}
	bad := []string{"", "a/b", "A", "Bild", "bild_1", "-bild", "bild-", "bild--1"}
	for _, s := range ok {
		if !domain.ArtifactSlugOK(s) {
			t.Errorf("ArtifactSlugOK(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if domain.ArtifactSlugOK(s) {
			t.Errorf("ArtifactSlugOK(%q) = true, want false", s)
		}
	}
}

func TestArtifact_IsImage(t *testing.T) {
	img := domain.Artifact{Mime: "image/png"}
	if !img.IsImage() {
		t.Error("image/png should be IsImage()")
	}
	pdf := domain.Artifact{Mime: "application/pdf"}
	if pdf.IsImage() {
		t.Error("application/pdf should not be IsImage()")
	}
}

func TestArtifactSlug(t *testing.T) {
	cases := map[string]string{
		"Mein Bild.PNG": "mein-bild",
		"doc.pdf":       "doc",
		"a b c":         "a-b-c",
		"straße.png":    "strasse",
	}
	for in, want := range cases {
		if got := domain.ArtifactSlug(in); got != want {
			t.Errorf("ArtifactSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
