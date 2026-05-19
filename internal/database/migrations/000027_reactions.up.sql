ALTER TABLE reactions
DROP CONSTRAINT reactions_reaction_type_check;

ALTER TABLE reactions
ADD CONSTRAINT reactions_reaction_type_check
CHECK (
	reaction_type IN (
		'heart',
		'laugh',
		'sad',
		'angry',
		'surprised',
		'fire',
		'clap',
		'thinking',
		'party',
		'cool'
	)
);