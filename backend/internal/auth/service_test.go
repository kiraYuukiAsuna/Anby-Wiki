package auth

import (
	"testing"

	"github.com/google/uuid"
)

type testIDGenerator struct{}

func (testIDGenerator) New() (uuid.UUID, error) {
	return uuid.New(), nil
}

func TestNewPersonalWikiIsIsolatedAndComplete(t *testing.T) {
	service := &Service{ids: testIDGenerator{}}
	firstOwner := uuid.New()
	secondOwner := uuid.New()

	first, err := service.newPersonalWiki(firstOwner, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.newPersonalWiki(secondOwner, "Second")
	if err != nil {
		t.Fatal(err)
	}

	if first.ID == second.ID || first.ComponentID == second.ComponentID {
		t.Fatal("personal Wiki resources share IDs")
	}
	if first.OwnerID != firstOwner || second.OwnerID != secondOwner {
		t.Fatal("personal Wiki owner was not preserved")
	}
	for _, wiki := range []personalWiki{first, second} {
		if wiki.ID == uuid.Nil || wiki.MainNamespaceID == uuid.Nil {
			t.Fatal("personal Wiki is missing required IDs")
		}
		if len(wiki.Namespaces) != len(personalNamespaceDefinitions) {
			t.Fatalf("namespace count=%d want=%d",
				len(wiki.Namespaces), len(personalNamespaceDefinitions))
		}
		if len(wiki.Tags) != len(personalChangeTagDefinitions) {
			t.Fatalf("change tag count=%d want=%d",
				len(wiki.Tags), len(personalChangeTagDefinitions))
		}
		mainCount := 0
		for _, namespace := range wiki.Namespaces {
			if namespace.Key == "main" {
				mainCount++
				if namespace.ID != wiki.MainNamespaceID {
					t.Fatal("main namespace ID does not match session value")
				}
			}
		}
		if mainCount != 1 {
			t.Fatalf("main namespace count=%d want=1", mainCount)
		}
	}
}

func TestValidateProfileUpdate(t *testing.T) {
	actorID := uuid.New()
	wikiID := uuid.New()

	email, normalizedEmail, displayName, wikiName, err := validateProfileUpdate(UpdateProfileInput{
		ActorID: actorID, WikiID: wikiID,
		Email:       " User@Example.COM ",
		DisplayName: "  User Name  ",
		WikiName:    "  Personal Wiki  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if email != "User@Example.COM" || normalizedEmail != "user@example.com" {
		t.Fatalf("email=%q normalized=%q", email, normalizedEmail)
	}
	if displayName != "User Name" || wikiName != "Personal Wiki" {
		t.Fatalf("displayName=%q wikiName=%q", displayName, wikiName)
	}

	_, _, _, _, err = validateProfileUpdate(UpdateProfileInput{
		ActorID: actorID, WikiID: wikiID,
		Email:       "invalid",
		DisplayName: "User",
		WikiName:    "Wiki",
	})
	if err == nil {
		t.Fatal("invalid email accepted")
	}

	_, _, _, _, err = validateProfileUpdate(UpdateProfileInput{
		ActorID: actorID, WikiID: wikiID,
		Email:       "user@example.com",
		DisplayName: " ",
		WikiName:    "Wiki",
	})
	if err == nil {
		t.Fatal("blank display name accepted")
	}
}
