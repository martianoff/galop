package native

// Entries are Repair.g4's entry rules for the tests, by name: Parse is the
// generated entry point, which repairs a failed SLL parse where the rule
// allows it, and Rerun parses the same rule but reruns the whole parse in LL
// mode after a failed SLL pass, as ANTLR does. Both give the tree as
// ToStringTree prints it.
var Entries = map[string]struct {
	Parse, Rerun func(*RepairTokens) (string, error)
}{
	"s1": {tree(ParseS1), rerun((*RepairGalopParser).ruleS1, RepairRULE_s1)},
	"s2": {tree(ParseS2), rerun((*RepairGalopParser).ruleS2, RepairRULE_s2)},
	"s3": {tree(ParseS3), rerun((*RepairGalopParser).ruleS3, RepairRULE_s3)},
	"s4": {tree(ParseS4), rerun((*RepairGalopParser).ruleS4, RepairRULE_s4)},
	"s5": {tree(ParseS5), rerun((*RepairGalopParser).ruleS5, RepairRULE_s5)},
	"s6": {tree(ParseS6), rerun((*RepairGalopParser).ruleS6, RepairRULE_s6)},
	"s7": {tree(ParseS7), rerun((*RepairGalopParser).ruleS7, RepairRULE_s7)},
	"s8": {tree(ParseS8), rerun((*RepairGalopParser).ruleS8, RepairRULE_s8)},
	"s9": {tree(ParseS9), rerun((*RepairGalopParser).ruleS9, RepairRULE_s9)},
	"s10": {tree(ParseS10), rerun((*RepairGalopParser).ruleS10, RepairRULE_s10)},
	"s11": {tree(ParseS11), rerun((*RepairGalopParser).ruleS11, RepairRULE_s11)},
	"s12": {tree(ParseS12), rerun((*RepairGalopParser).ruleS12, RepairRULE_s12)},
}

func tree[N interface{ ToStringTree() string }](parse func(*RepairTokens) (N, error)) func(*RepairTokens) (string, error) {
	return func(ts *RepairTokens) (string, error) {
		n, err := parse(ts)
		if err != nil {
			return "", err
		}
		return n.ToStringTree(), nil
	}
}

func rerun(rule func(*RepairGalopParser) int32, index int) func(*RepairTokens) (string, error) {
	return func(ts *RepairTokens) (string, error) {
		t, root, err := newRepairGalopParser(ts).run(rule, RepairGalopRuleStartState[index], false)
		if err != nil {
			return "", err
		}
		return RepairNodeRef{t, root}.ToStringTree(), nil
	}
}
