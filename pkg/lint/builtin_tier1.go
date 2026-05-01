package lint

func init() {
	RegisterBuiltin("check_step_numbering", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkStepNumbering(ctx.Parsed)
	})

	RegisterBuiltin("check_uuid_unique", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkUUIDUnique(ctx.Parsed)
	})

	RegisterBuiltin("check_uuid_descriptive", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkUUIDDescriptive(ctx.Parsed)
	})

	RegisterBuiltin("check_trigger_number_zero", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkTriggerNumberZero(ctx.Parsed)
	})

	RegisterBuiltin("check_filename_match", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkFilenameMatch(ctx.Parsed, ctx.Filename)
	})

	RegisterBuiltin("check_config_no_workato", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkConfigNoWorkato(ctx.Parsed)
	})

	RegisterBuiltin("check_config_provider_match", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkConfigProviderMatch(ctx.Parsed, ctx.ConnRules)
	})

	RegisterBuiltin("check_action_name_valid", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return checkActionNameValid(ctx.Parsed, ctx.ConnRules)
	})

	RegisterBuiltin("check_action_rules", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		allDiags := ctx.CacheGetOrCompute("check_action_rules", func() interface{} {
			return checkActionRules(ctx.Parsed, ctx.ConnRules)
		}).([]LintDiagnostic)
		return allDiags
	})

	RegisterBuiltin("check_datapills", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		allDiags := ctx.CacheGetOrCompute("check_datapills", func() interface{} {
			return checkDatapillsWithCatchAliases(ctx.Parsed, ctx.ConnRules)
		}).([]LintDiagnostic)
		var filtered []LintDiagnostic
		for _, d := range allDiags {
			if d.RuleID == rule.RuleID {
				filtered = append(filtered, d)
			}
		}
		return filtered
	})

	RegisterBuiltin("check_eis", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		allDiags := ctx.CacheGetOrCompute("check_eis", func() interface{} {
			return checkEIS(ctx.Parsed, ctx.ConnRules)
		}).([]LintDiagnostic)
		var filtered []LintDiagnostic
		for _, d := range allDiags {
			if d.RuleID == rule.RuleID {
				filtered = append(filtered, d)
			}
		}
		return filtered
	})

	RegisterBuiltin("check_formulas", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		allDiags := ctx.CacheGetOrCompute("check_formulas", func() interface{} {
			return checkFormulaMethods(ctx.Parsed)
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
