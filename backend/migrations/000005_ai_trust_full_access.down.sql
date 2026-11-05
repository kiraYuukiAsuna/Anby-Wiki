UPDATE ai_trust_profile
SET trust_level='trusted', required_sample_percent=0
WHERE trust_level='full_access';

ALTER TABLE ai_trust_profile
    DROP CONSTRAINT IF EXISTS ai_trust_profile_trust_level_check;

ALTER TABLE ai_trust_profile
    ADD CONSTRAINT ai_trust_profile_trust_level_check
    CHECK (trust_level IN ('untrusted','assisted','trusted'));
