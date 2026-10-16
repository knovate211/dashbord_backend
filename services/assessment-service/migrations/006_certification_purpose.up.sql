-- Certification exams are a fourth kind of assessment.
--
-- Like scholarship and hiring papers they are invite-only and decide an outcome,
-- but the outcome is a credential the candidate paid for rather than a discount
-- or a hiring decision. Keeping them apart from 'scholarship' matters because
-- every scholarship screen, export and funnel count joins on that purpose; a
-- paid exam appearing there would corrupt the scholarship numbers the same way
-- a companyless row would corrupt the recruiter reports.
--
-- Nothing else is needed to make the new purpose invite-only: inviteOnly() in
-- assessment-service is a denylist of one ('practice'), so an unknown purpose
-- fails closed.
ALTER TABLE assessments DROP CONSTRAINT IF EXISTS assessments_purpose_check;
ALTER TABLE assessments ADD  CONSTRAINT assessments_purpose_check
    CHECK (purpose IN ('practice', 'hiring', 'scholarship', 'certification'));
