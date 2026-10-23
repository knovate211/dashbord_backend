-- Every problem load reads its examples, hints and constraints by problem_id,
-- ordered by order_index, and deleting a problem cascades to all three. None
-- of them had an index on problem_id, so each read was a full table scan.
CREATE INDEX IF NOT EXISTS idx_examples_problem    ON examples(problem_id, order_index);
CREATE INDEX IF NOT EXISTS idx_hints_problem       ON hints(problem_id, order_index);
CREATE INDEX IF NOT EXISTS idx_constraints_problem ON problem_constraints(problem_id, order_index);
