package component

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRenderEntityInfoboxGroupsValuesAndShowsReadableMetadata(t *testing.T) {
	entityID := uuid.MustParse("019ff608-7581-77f4-ad85-9aa7862c28f2")
	firstAuthorID := uuid.MustParse("019ff608-7581-77f4-ad85-9aa7862c2801")
	secondAuthorID := uuid.MustParse("019ff608-7581-77f4-ad85-9aa7862c2802")
	issuerID := uuid.MustParse("019ff608-7581-77f4-ad85-9aa7862c2803")
	rows := []infoboxClaim{
		{claimID: uuid.New(), propertyKey: "author", propertyName: "作者", value: "Michael B. Jones", targetEntityID: &firstAuthorID, verificationStatus: "unverified"},
		{claimID: uuid.New(), propertyKey: "issued_by", propertyName: "发布组织", value: "Internet Engineering Task Force", targetEntityID: &issuerID, verificationStatus: "disputed"},
		{claimID: uuid.New(), propertyKey: "document_identifier", propertyName: "文档编号", value: "RFC 7523", verificationStatus: "human_verified"},
		{claimID: uuid.New(), propertyKey: "author", propertyName: "作者", value: "Brian Campbell", targetEntityID: &secondAuthorID, verificationStatus: "unverified"},
		// The same Entity may be supported by more than one published Claim. The
		// compact infobox should list the value once, while claim_usage can still
		// retain every underlying Claim ID.
		{claimID: uuid.New(), propertyKey: "author", propertyName: "作者", value: "M. B. Jones", targetEntityID: &firstAuthorID, verificationStatus: "unverified"},
	}

	got := renderEntityInfobox(entityID, infoboxEntity{
		label: "JSON Web Token (JWT) Profile <RFC>", typeKey: "work", typeName: "作品",
	}, rows, infoboxConfig{})

	if strings.Count(got, "<dt>作者</dt>") != 1 {
		t.Fatalf("authors were not grouped into one row: %s", got)
	}
	if strings.Count(got, firstAuthorID.String()) != 1 {
		t.Fatalf("duplicate author target was not collapsed: %s", got)
	}
	for _, want := range []string{
		`data-entity-type="work"`,
		`<p class="entity-infobox-type">作品</p>`,
		`JSON Web Token (JWT) Profile &lt;RFC&gt;`,
		`href="/entities/` + secondAuthorID.String() + `"`,
		`href="/entities/` + issuerID.String() + `"`,
		`1 条事实存在争议 · 2 条事实待核验`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered infobox does not contain %q: %s", want, got)
		}
	}
	identifierIndex := strings.Index(got, `data-property-key="document_identifier"`)
	authorIndex := strings.Index(got, `data-property-key="author"`)
	issuerIndex := strings.Index(got, `data-property-key="issued_by"`)
	if identifierIndex < 0 || authorIndex < 0 || issuerIndex < 0 || !(identifierIndex < authorIndex && authorIndex < issuerIndex) {
		t.Fatalf("work metadata ordering is unstable: %s", got)
	}
}

func TestInfoboxValueTextUsesReadableCanonicalKeyFallback(t *testing.T) {
	targetID := uuid.New()
	canonicalKey := "person:michael b. jones"
	got, err := infoboxValueText("entity", json.RawMessage(`{"entity_id":"`+targetID.String()+`"}`), &targetID, &canonicalKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "michael b. jones" {
		t.Fatalf("fallback label=%q, want canonical key without type prefix", got)
	}
	if got := displayCanonicalKey("work:json web token"); got != "json web token" {
		t.Fatalf("displayCanonicalKey=%q", got)
	}
}
