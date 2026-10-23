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
	prepared := make([]EntityCandidate, 0, len(input.Entities))
	for _, raw := range input.Entities {
		if raw.CandidateID == uuid.Nil || strings.TrimSpace(raw.TypeKey) == "" || strings.TrimSpace(raw.Label) == "" {
			continue
		}
		candidate := raw
		candidate.Aliases = mergeAliases(nil, candidate.Aliases)
		candidate.Evidence = mergeCandidateEvidence(nil, candidate.Evidence)
		prepared = append(prepared, candidate)
	}
	remappedIDs := make(map[uuid.UUID]uuid.UUID, len(prepared))
	for _, candidate := range prepared {
		if index, ok := matchingEntityCandidate(result.Entities, candidate, prepared); ok {
			canonicalID := result.Entities[index].CandidateID
			remappedIDs[candidate.CandidateID] = canonicalID
			mergeEntityCandidate(&result.Entities[index], candidate)
			continue
		}
		remappedIDs[candidate.CandidateID] = candidate.CandidateID
		result.Entities = append(result.Entities, candidate)
	}

	types := make(map[uuid.UUID]string, len(result.Entities))
	entities := make(map[uuid.UUID]EntityCandidate, len(result.Entities))
	for _, candidate := range result.Entities {
		types[candidate.CandidateID] = normalizedPlanText(candidate.TypeKey)
		entities[candidate.CandidateID] = candidate
	}
	claimKeys := map[string]bool{}
	for _, raw := range input.Claims {
		candidate := raw
		if !remapClaimCandidate(&candidate, remappedIDs) || claimCandidateSelfReferential(candidate) ||
			!claimCandidateTypeSafe(candidate, types) || !claimCandidateRelationExplicit(candidate, entities) {
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

// claimCandidateRelationExplicit applies conservative evidence guards to
// semantic relations that models commonly infer from nearby work mentions.
// Page prose is retained when a Claim is dropped, so requiring an explicit
// directional phrase improves graph precision without losing source content.
func claimCandidateRelationExplicit(candidate ClaimCandidate, entities map[uuid.UUID]EntityCandidate) bool {
	if candidate.PropertyKey != "instance_of" &&
		candidate.PropertyKey != "updates" &&
		candidate.PropertyKey != "obsoletes" {
		return true
	}
	if candidate.Subject.CandidateID != nil {
		valueID, referencesCandidate, valid := claimValueCandidateReference(candidate.Value)
		subject, subjectFound := entities[*candidate.Subject.CandidateID]
		value, valueFound := entities[valueID]
		if !valid || !referencesCandidate || !subjectFound || !valueFound {
			return false
		}
		for _, item := range candidate.Evidence {
			if candidate.PropertyKey == "instance_of" &&
				evidenceExplicitlyClassifies(item.Quotation, subject, value) {
				return true
			}
			if candidate.PropertyKey != "instance_of" &&
				claimEvidenceExplicitlyRelatesWorks(
					item, subject, value, candidate.PropertyKey, entities,
				) {
				return true
			}
		}
		return false
	}
	if candidate.PropertyKey != "instance_of" {
		return false
	}
	for _, item := range candidate.Evidence {
		text := " " + strings.ToLower(strings.Join(strings.Fields(item.Quotation), " ")) + " "
		for _, marker := range []string{
			" is a ", " is an ", " is the ", " are a ", " are an ",
			" type of ", " kind of ", " class of ", " instance of ",
			"是一种", "是一个", "是一类", "属于一种", "属于一类", "为一种",
		} {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

func claimEvidenceExplicitlyRelatesWorks(
	evidence CandidateEvidence,
	subject EntityCandidate,
	value EntityCandidate,
	property string,
	entities map[uuid.UUID]EntityCandidate,
) bool {
	if evidenceExplicitlyRelatesWorks(evidence.Quotation, subject, value, property) {
		return true
	}
	if !pronounEvidenceExplicitlyRelatesValue(evidence.Quotation, value, property) {
		return false
	}

	const maximumSubjectDistance = 240
	subjectEnd := -1
	for _, item := range subject.Evidence {
		if item.ChunkID != evidence.ChunkID || item.CharEnd > evidence.CharStart ||
			evidence.CharStart-item.CharEnd > maximumSubjectDistance ||
			!evidenceNamesEntity(item.Quotation, subject) {
			continue
		}
		subjectEnd = max(subjectEnd, item.CharEnd)
	}
	if subjectEnd < 0 {
		return false
	}
	for _, other := range entities {
		if other.CandidateID == subject.CandidateID || other.CandidateID == value.CandidateID ||
			normalizedPlanText(other.TypeKey) != "work" {
			continue
		}
		for _, item := range other.Evidence {
			if item.ChunkID == evidence.ChunkID && item.CharEnd > subjectEnd &&
				item.CharStart < evidence.CharStart {
				return false
			}
		}
	}
	return true
}

func pronounEvidenceExplicitlyRelatesValue(
	quotation string,
	value EntityCandidate,
	property string,
) bool {
	text := normalizedIdentityText(quotation)
	for valueName := range entityCandidateNames(value) {
		patterns := []string{"it updates " + valueName}
		if property == "obsoletes" {
			patterns = []string{
				"it obsoletes " + valueName,
				"it supersedes " + valueName,
				"it replaces " + valueName,
			}
		}
		patterns = append(patterns, "it updates and obsoletes "+valueName)
		for _, pattern := range patterns {
			if text == pattern || strings.HasPrefix(text, pattern+" ") {
				return true
			}
		}
	}
	return false
}

func evidenceNamesEntity(quotation string, entity EntityCandidate) bool {
	text := " " + normalizedIdentityText(quotation) + " "
	for name := range entityCandidateNames(entity) {
		if strings.Contains(text, " "+name+" ") {
			return true
		}
	}
	return false
}

func evidenceExplicitlyRelatesWorks(
	quotation string,
	subject EntityCandidate,
	value EntityCandidate,
	property string,
) bool {
	markers := []string{"updates"}
	if property == "obsoletes" {
		markers = []string{"obsoletes", "supersedes", "replaces"}
	}
	text := " " + normalizedIdentityText(quotation) + " "
	for subjectName := range entityCandidateNames(subject) {
		for valueName := range entityCandidateNames(value) {
			for _, marker := range markers {
				if strings.Contains(
					text,
					" "+subjectName+" "+marker+" "+valueName+" ",
				) || strings.Contains(
					text,
					" "+subjectName+" updates and obsoletes "+valueName+" ",
				) {
					return true
				}
			}
		}
	}
	compact := strings.ReplaceAll(text, " ", "")
	for subjectName := range entityCandidateNames(subject) {
		for valueName := range entityCandidateNames(value) {
			subjectName = strings.ReplaceAll(subjectName, " ", "")
			valueName = strings.ReplaceAll(valueName, " ", "")
			markers := []string{"更新", "修订"}
			if property == "obsoletes" {
				markers = []string{"废止", "取代", "替代"}
			}
			for _, marker := range markers {
				if strings.Contains(
					compact,
					subjectName+marker+valueName,
				) {
					return true
				}
			}
		}
	}
	return false
}

func evidenceExplicitlyClassifies(quotation string, subject, value EntityCandidate) bool {
	text := " " + normalizedIdentityText(quotation) + " "
	for subjectName := range entityCandidateNames(subject) {
		for valueName := range entityCandidateNames(value) {
			for _, marker := range []string{
				" is a type of ", " is a kind of ", " is an instance of ",
				" is a ", " is an ", " is the ", " are a ", " are an ",
			} {
				if strings.Contains(text, " "+subjectName+marker+valueName+" ") {
					return true
				}
			}
			compact := strings.ReplaceAll(text, " ", "")
			for _, marker := range []string{"是一种", "是一个", "是一类", "属于一种", "属于一类", "为一种"} {
				if strings.Contains(compact, strings.ReplaceAll(subjectName, " ", "")+
					marker+strings.ReplaceAll(valueName, " ", "")) {
					return true
				}
			}
		}
	}
	return false
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

func matchingEntityCandidate(existing []EntityCandidate, incoming EntityCandidate, all []EntityCandidate) (int, bool) {
	matches := make([]int, 0, 2)
	for index := range existing {
		if entityCandidatesEquivalent(existing[index], incoming) ||
			personAbbreviationMergeAllowed(existing[index], incoming, all) {
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
	return false
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
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#'
	})
	return strings.Join(parts, " ")
}

func personAbbreviationMergeAllowed(left, right EntityCandidate, all []EntityCandidate) bool {
	if normalizedPlanText(left.TypeKey) != "person" ||
		normalizedPlanText(right.TypeKey) != "person" {
		return false
	}
	if !personCandidatesCompatible(left, right) {
		return false
	}
	if personCandidateHasAbbreviatedName(left) == personCandidateHasAbbreviatedName(right) {
		return false
	}
	abbreviated := left
	full := right
	if personCandidateHasAbbreviatedName(right) {
		abbreviated, full = right, left
	}
	if !personCandidateHasFullCompatibleName(full, abbreviated) {
		return false
	}
	compatibleFullCandidates := map[uuid.UUID]bool{}
	for _, candidate := range all {
		if normalizedPlanText(candidate.TypeKey) != "person" ||
			!personCandidateHasFullCompatibleName(candidate, abbreviated) {
			continue
		}
		compatibleFullCandidates[candidate.CandidateID] = true
	}
	return len(compatibleFullCandidates) == 1
}

func personCandidatesCompatible(left, right EntityCandidate) bool {
	for _, leftName := range entityPersonNames(left) {
		for _, rightName := range entityPersonNames(right) {
			if personNamesCompatible(leftName, rightName) {
				return true
			}
		}
	}
	return false
}

func personCandidateHasFullCompatibleName(candidate, abbreviated EntityCandidate) bool {
	for _, fullName := range entityPersonNames(candidate) {
		if !personNameHasFullGiven(fullName) {
			continue
		}
		for _, shortName := range entityPersonNames(abbreviated) {
			if personNameHasAbbreviatedGiven(shortName) &&
				personNamesCompatible(fullName, shortName) {
				return true
			}
		}
	}
	return false
}

func personCandidateHasAbbreviatedName(candidate EntityCandidate) bool {
	for _, name := range entityPersonNames(candidate) {
		if personNameHasAbbreviatedGiven(name) {
			return true
		}
	}
	return false
}

func entityPersonNames(candidate EntityCandidate) []string {
	values := append([]string{candidate.Label}, candidate.Aliases...)
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		key := normalizedIdentityText(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func personNameHasAbbreviatedGiven(name string) bool {
	parts := strings.Fields(normalizedIdentityText(name))
	return len(parts) >= 2 && utf8.RuneCountInString(parts[0]) == 1
}

func personNameHasFullGiven(name string) bool {
	parts := strings.Fields(normalizedIdentityText(name))
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if utf8.RuneCountInString(part) > 1 {
			return true
		}
	}
	return false
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
	selected := map[uuid.UUID]bool{}
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		title := normalizedIdentityText(route.Title)
		exact := make([]uuid.UUID, 0, 1)
		for _, candidate := range candidates.Entities {
			if normalizedIdentityText(candidate.Label) == title {
				exact = append(exact, candidate.CandidateID)
			}
		}
		if len(exact) == 1 {
			selected[exact[0]] = true
			continue
		}
		if len(exact) > 1 {
			continue
		}
		wanted := map[string]bool{}
		addPlanIdentityKeys(wanted, route.Title)
		for _, candidate := range candidates.Entities {
			for name := range entityCandidateNames(candidate) {
				if wanted[name] {
					selected[candidate.CandidateID] = true
					break
				}
			}
		}
	}
	byID := make(map[uuid.UUID]EntityCandidate, len(candidates.Entities))
	for _, candidate := range candidates.Entities {
		byID[candidate.CandidateID] = candidate
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
