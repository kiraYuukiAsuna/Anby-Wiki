package importer

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

func rankedEntityCandidates(candidates *Candidates, limit int) []EntityCandidate {
	if candidates == nil || limit <= 0 {
		return nil
	}
	result := append([]EntityCandidate(nil), candidates.Entities...)
	sort.SliceStable(result, func(i, j int) bool {
		if len(result[i].Evidence) != len(result[j].Evidence) {
			return len(result[i].Evidence) > len(result[j].Evidence)
		}
		if result[i].Confidence != result[j].Confidence {
			return result[i].Confidence > result[j].Confidence
		}
		return normalizedIdentityText(result[i].Label) < normalizedIdentityText(result[j].Label)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// normalizeCandidatesForUse repairs cross-batch identity drift without
// mutating the immutable extraction row. Models commonly emit an abbreviated
// person or organization in one source window and its full name in another;
// treating those spellings as independent Entities creates duplicate graph
// nodes and can also duplicate Claims that refer to them.
func normalizeCandidatesForUse(input *Candidates) *Candidates {
	if input == nil {
		return nil
	}
	result := &Candidates{
		SchemaVersion: input.SchemaVersion, SourceVersionID: input.SourceVersionID,
		Entities: []EntityCandidate{}, Claims: []ClaimCandidate{},
		QualityScore: input.QualityScore, PromptInjectionDetected: input.PromptInjectionDetected,
	}
	remappedIDs := make(map[uuid.UUID]uuid.UUID, len(input.Entities))
	for _, raw := range input.Entities {
		if raw.CandidateID == uuid.Nil || strings.TrimSpace(raw.TypeKey) == "" || strings.TrimSpace(raw.Label) == "" {
			continue
		}
		candidate := raw
		candidate.Aliases = mergeAliases(nil, candidate.Aliases)
		candidate.Evidence = mergeCandidateEvidence(nil, candidate.Evidence)
		if index, ok := matchingEntityCandidate(result.Entities, candidate); ok {
			canonicalID := result.Entities[index].CandidateID
			remappedIDs[raw.CandidateID] = canonicalID
			mergeEntityCandidate(&result.Entities[index], candidate)
			continue
		}
		remappedIDs[raw.CandidateID] = candidate.CandidateID
		result.Entities = append(result.Entities, candidate)
	}

	types := make(map[uuid.UUID]string, len(result.Entities))
	for _, candidate := range result.Entities {
		types[candidate.CandidateID] = normalizedPlanText(candidate.TypeKey)
	}
	claimKeys := map[string]bool{}
	for _, raw := range input.Claims {
		candidate := raw
		if !remapClaimCandidate(&candidate, remappedIDs) || claimCandidateSelfReferential(candidate) ||
			!claimCandidateTypeSafe(candidate, types) {
			continue
		}
		candidate.Evidence = mergeCandidateEvidence(nil, candidate.Evidence)
		key, err := normalizedClaimKey(candidate)
		if err != nil || claimKeys[key] {
			continue
		}
		claimKeys[key] = true
		result.Claims = append(result.Claims, candidate)
	}
	return result
}

func claimCandidateSelfReferential(candidate ClaimCandidate) bool {
	var value struct {
		EntityCandidateID *uuid.UUID `json:"entity_candidate_id"`
		EntityID          *uuid.UUID `json:"entity_id"`
	}
	if json.Unmarshal(candidate.Value, &value) != nil {
		return false
	}
	if candidate.Subject.CandidateID != nil && value.EntityCandidateID != nil {
		return *candidate.Subject.CandidateID == *value.EntityCandidateID
	}
	if candidate.Subject.EntityID != nil && value.EntityID != nil {
		return *candidate.Subject.EntityID == *value.EntityID
	}
	return false
}

func matchingEntityCandidate(existing []EntityCandidate, incoming EntityCandidate) (int, bool) {
	matches := make([]int, 0, 2)
	for index := range existing {
		if entityCandidatesEquivalent(existing[index], incoming) {
			matches = append(matches, index)
		}
	}
	// An abbreviated name is merged only when it has one unambiguous typed
	// destination. "M. Jones" must not collapse two distinct full names.
	return func() (int, bool) {
		if len(matches) != 1 {
			return 0, false
		}
		return matches[0], true
	}()
}

func entityCandidatesEquivalent(left, right EntityCandidate) bool {
	if normalizedPlanText(left.TypeKey) != normalizedPlanText(right.TypeKey) {
		return false
	}
	leftNames, rightNames := entityCandidateNames(left), entityCandidateNames(right)
	for name := range leftNames {
		if rightNames[name] {
			return true
		}
	}
	return normalizedPlanText(left.TypeKey) == "person" && personNamesCompatible(left.Label, right.Label)
}

func entityCandidateNames(candidate EntityCandidate) map[string]bool {
	result := map[string]bool{}
	for _, value := range append([]string{candidate.Label}, candidate.Aliases...) {
		if key := normalizedIdentityText(value); key != "" {
			result[key] = true
		}
	}
	return result
}

func normalizedIdentityText(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(parts, " ")
}

func personNamesCompatible(left, right string) bool {
	a, b := strings.Fields(normalizedIdentityText(left)), strings.Fields(normalizedIdentityText(right))
	if len(a) < 2 || len(b) < 2 || a[len(a)-1] != b[len(b)-1] {
		return false
	}
	aGiven, bGiven := a[:len(a)-1], b[:len(b)-1]
	if !abbreviatedGivenNames(aGiven) && !abbreviatedGivenNames(bGiven) {
		return false
	}
	limit := min(len(aGiven), len(bGiven))
	for index := 0; index < limit; index++ {
		if firstRune(aGiven[index]) != firstRune(bGiven[index]) {
			return false
		}
		if utf8.RuneCountInString(aGiven[index]) > 1 && utf8.RuneCountInString(bGiven[index]) > 1 &&
			aGiven[index] != bGiven[index] {
			return false
		}
	}
	return limit > 0
}

func abbreviatedGivenNames(values []string) bool {
	for _, value := range values {
		if utf8.RuneCountInString(value) == 1 {
			return true
		}
	}
	return false
}

func firstRune(value string) rune {
	result, _ := utf8.DecodeRuneInString(value)
	return result
}

func mergeEntityCandidate(existing *EntityCandidate, incoming EntityCandidate) {
	if existing == nil {
		return
	}
	aliases := append([]string{}, existing.Aliases...)
	if preferEntityLabel(incoming.TypeKey, incoming.Label, existing.Label) {
		aliases = append(aliases, existing.Label)
		existing.Label = incoming.Label
	} else {
		aliases = append(aliases, incoming.Label)
	}
	aliases = append(aliases, incoming.Aliases...)
	existing.Aliases = mergeAliases(nil, aliases)
	filtered := existing.Aliases[:0]
	for _, alias := range existing.Aliases {
		if !strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(existing.Label)) {
			filtered = append(filtered, alias)
		}
	}
	existing.Aliases = filtered
	existing.Evidence = mergeCandidateEvidence(existing.Evidence, incoming.Evidence)
	existing.Confidence = max(existing.Confidence, incoming.Confidence)
}

func preferEntityLabel(typeKey, incoming, existing string) bool {
	typeKey = normalizedPlanText(typeKey)
	if typeKey == "person" {
		existingParts := strings.Fields(normalizedIdentityText(existing))
		incomingParts := strings.Fields(normalizedIdentityText(incoming))
		if len(existingParts) < 2 || len(incomingParts) < 2 {
			return false
		}
		return personLabelInformation(incomingParts[:len(incomingParts)-1]) >
			personLabelInformation(existingParts[:len(existingParts)-1])
	}
	if typeKey == "organization" {
		return looksLikeAcronym(existing) && !looksLikeAcronym(incoming)
	}
	return false
}

func personLabelInformation(givenNames []string) int {
	score := 0
	for _, value := range givenNames {
		length := utf8.RuneCountInString(value)
		if length > 1 {
			score += length
		}
	}
	return score
}

func looksLikeAcronym(value string) bool {
	letters, lower := 0, false
	for _, r := range strings.TrimSpace(value) {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		lower = lower || unicode.IsLower(r)
	}
	return letters >= 2 && letters <= 12 && !lower
}

func remapClaimCandidate(candidate *ClaimCandidate, remapped map[uuid.UUID]uuid.UUID) bool {
	if candidate == nil || candidate.CandidateID == uuid.Nil {
		return false
	}
	if candidate.Subject.CandidateID != nil {
		mapped, ok := remapped[*candidate.Subject.CandidateID]
		if !ok {
			return false
		}
		candidate.Subject.CandidateID = &mapped
	}
	valueID, referencesCandidate, valid := claimValueCandidateReference(candidate.Value)
	if !valid {
		return false
	}
	if referencesCandidate {
		mapped, ok := remapped[valueID]
		if !ok {
			return false
		}
		candidate.Value, _ = json.Marshal(map[string]uuid.UUID{"entity_candidate_id": mapped})
	}
	return true
}

// claimCandidateTypeSafe is a deterministic backstop for relation direction.
// It deliberately drops an unsupported relation instead of guessing a repair;
// exact evidence proves the words, but not that a model chose the right
// subject/property/value semantics.
func claimCandidateTypeSafe(candidate ClaimCandidate, types map[uuid.UUID]string) bool {
	subjectType := ""
	if candidate.Subject.CandidateID != nil {
		subjectType = types[*candidate.Subject.CandidateID]
		if subjectType == "" {
			return false
		}
	}
	valueType := ""
	if valueID, referencesCandidate, valid := claimValueCandidateReference(candidate.Value); !valid {
		return false
	} else if referencesCandidate {
		valueType = types[valueID]
		if valueType == "" {
			return false
		}
	}
	// Existing authoritative Entity IDs are checked by the classifier and
	// knowledge service. Apply every constraint for which the candidate type is
	// known, without guessing the missing side.
	switch normalizedPlanText(candidate.PropertyKey) {
	case "author":
		return (subjectType == "" || subjectType == "work") &&
			(valueType == "" || oneOf(valueType, "person", "organization"))
	case "developer":
		return (subjectType == "" || oneOf(subjectType, "software", "product")) &&
			(valueType == "" || oneOf(valueType, "person", "organization"))
	case "manufacturer":
		return (subjectType == "" || subjectType == "product") &&
			(valueType == "" || valueType == "organization")
	case "voice_actor":
		return (subjectType == "" || subjectType == "character") &&
			(valueType == "" || valueType == "person")
	case "located_in":
		return (subjectType == "" || oneOf(subjectType, "place", "organization", "event")) &&
			(valueType == "" || valueType == "place")
	case "part_of":
		// A work can belong to a work/series, but an issuing or publishing
		// organization is not its container. Keep this deterministic guard even
		// when a provider ignores the prompt's issued_by instruction.
		if subjectType == "work" && valueType == "organization" {
			return false
		}
		return (subjectType == "" || oneOf(subjectType, "organization", "place", "work", "event", "product", "concept", "software", "species")) &&
			(valueType == "" || oneOf(valueType, "organization", "place", "work", "event", "product", "concept", "software"))
	case "issued_by":
		return (subjectType == "" || subjectType == "work") &&
			(valueType == "" || valueType == "organization")
	case "updates", "obsoletes":
		return (subjectType == "" || subjectType == "work") &&
			(valueType == "" || valueType == "work")
	case "document_identifier", "document_category", "document_status":
		return subjectType == "" || subjectType == "work"
	case "instance_of":
		return valueType == "" || oneOf(valueType, "concept", "species")
	case "release_date":
		return subjectType == "" || oneOf(subjectType, "work", "product", "software")
	default:
		return true
	}
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

// selectCandidatesForPlan prevents an extraction batch from turning every
// bibliographic mention into an authoritative graph write. Actionable page
// routes choose the graph seeds; only a valid Claim whose subject is selected
// may pull in an additional value Entity dependency. The source profile is a
// discovery aid, not write authority: otherwise incidental authors and cited
// works become disconnected Entities even when they never received a route.
func selectCandidatesForPlan(candidates *Candidates, plan *ImportPlan) *Candidates {
	if candidates == nil {
		return nil
	}
	result := &Candidates{
		SchemaVersion: candidates.SchemaVersion, SourceVersionID: candidates.SourceVersionID,
		Entities: []EntityCandidate{}, Claims: []ClaimCandidate{},
		QualityScore: candidates.QualityScore, PromptInjectionDetected: candidates.PromptInjectionDetected,
	}
	if plan == nil {
		return result
	}
	wanted := map[string]bool{}
	for _, route := range plan.Routes {
		if route.Action == RouteCreate || route.Action == RouteUpdate {
			addPlanIdentityKeys(wanted, route.Title)
		}
	}
	selected := map[uuid.UUID]bool{}
	byID := make(map[uuid.UUID]EntityCandidate, len(candidates.Entities))
	for _, candidate := range candidates.Entities {
		byID[candidate.CandidateID] = candidate
		for name := range entityCandidateNames(candidate) {
			if wanted[name] {
				selected[candidate.CandidateID] = true
				break
			}
		}
	}
	for _, candidate := range candidates.Claims {
		if candidate.Subject.CandidateID == nil || !selected[*candidate.Subject.CandidateID] {
			continue
		}
		if valueID, referencesCandidate, valid := claimValueCandidateReference(candidate.Value); !valid {
			continue
		} else if referencesCandidate {
			if _, ok := byID[valueID]; !ok {
				continue
			}
			selected[valueID] = true
		}
		result.Claims = append(result.Claims, candidate)
	}
	for _, candidate := range candidates.Entities {
		if selected[candidate.CandidateID] {
			result.Entities = append(result.Entities, candidate)
		}
	}
	return result
}

func addPlanIdentityKeys(destination map[string]bool, value string) {
	if destination == nil {
		return
	}
	if key := normalizedIdentityText(value); key != "" {
		destination[key] = true
	}
	depth := 0
	var builder strings.Builder
	for _, r := range value {
		switch r {
		case '(', '（':
			depth++
		case ')', '）':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				builder.WriteRune(r)
			}
		}
	}
	if key := normalizedIdentityText(builder.String()); key != "" {
		destination[key] = true
	}
}
