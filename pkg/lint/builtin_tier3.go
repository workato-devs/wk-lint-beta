package lint

func init() {
	RegisterBuiltin("check_dataflow", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		allDiags := ctx.CacheGetOrCompute("check_dataflow", func() interface{} {
			return lintTier3DataFlow(ctx.Parsed, ctx.Graph)
		}).([]LintDiagnostic)
		var filtered []LintDiagnostic
		for _, d := range allDiags {
			if d.RuleID == rule.RuleID {
				filtered = append(filtered, d)
			}
		}
		return filtered
	})
}
