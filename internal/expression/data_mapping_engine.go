package expression

// MappingResult contains the result of processing a record through the mapping engine.
type MappingResult struct {
	// RulesMatched is the number of rules whose conditions matched.
	RulesMatched int

	// TotalModifications is the total number of field modifications made.
	TotalModifications int

	// AllModifications contains details of all modifications.
	AllModifications []FieldModification
}

// DataMappingEngine processes records through a series of rules.
type DataMappingEngine struct {
	rules            []*ParsedExpression
	ruleProcessor    *RuleProcessor
	stopOnFirstMatch bool
}

// NewDataMappingEngine creates a new data mapping engine.
func NewDataMappingEngine() *DataMappingEngine {
	return &DataMappingEngine{
		rules:         make([]*ParsedExpression, 0),
		ruleProcessor: NewRuleProcessor(),
	}
}

// AddRule adds a parsed rule to the engine.
func (e *DataMappingEngine) AddRule(rule *ParsedExpression) {
	e.rules = append(e.rules, rule)
}

// AddRuleString parses and adds a rule string to the engine.
func (e *DataMappingEngine) AddRuleString(rule string) error {
	parsed, err := Parse(rule)
	if err != nil {
		return err
	}
	e.AddRule(parsed)
	return nil
}

// ClearRules removes all rules from the engine.
func (e *DataMappingEngine) ClearRules() {
	e.rules = make([]*ParsedExpression, 0)
}

// SetStopOnFirstMatch configures whether to stop after the first matching rule.
func (e *DataMappingEngine) SetStopOnFirstMatch(stop bool) {
	e.stopOnFirstMatch = stop
}

// Process processes a record through all rules in order.
func (e *DataMappingEngine) Process(ctx ModifiableContext) (*MappingResult, error) {
	result := &MappingResult{
		AllModifications: make([]FieldModification, 0),
	}

	for _, rule := range e.rules {
		ruleResult, err := e.ruleProcessor.Apply(rule, ctx)
		if err != nil {
			return nil, err
		}

		if ruleResult.Matched {
			result.RulesMatched++
			result.TotalModifications += len(ruleResult.Modifications)
			result.AllModifications = append(result.AllModifications, ruleResult.Modifications...)

			if e.stopOnFirstMatch {
				break
			}
		}
	}

	return result, nil
}

// RuleCount returns the number of rules in the engine.
func (e *DataMappingEngine) RuleCount() int {
	return len(e.rules)
}
