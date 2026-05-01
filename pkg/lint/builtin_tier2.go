package lint

func init() {
	RegisterBuiltin("check_catch_last_in_try", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkCatchLastInTry(ctx.Graph)
	})

	RegisterBuiltin("check_else_last_in_if", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkElseLastInIf(ctx.Graph)
	})

	RegisterBuiltin("check_success_before_catch", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkSuccessBeforeCatch(ctx.Graph, ctx.Parsed)
	})

	RegisterBuiltin("check_terminal_coverage", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkTerminalCoverage(ctx.Graph, ctx.Parsed)
	})

	RegisterBuiltin("check_all_paths_return", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkAllPathsReturn(ctx.Graph)
	})

	RegisterBuiltin("check_catch_returns_all_fields", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkCatchReturnsAllFields(ctx.Graph, ctx.Parsed)
	})

	RegisterBuiltin("check_recipe_call_zip_name", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		if ctx.Graph == nil {
			return nil
		}
		return checkRecipeCallZipName(ctx.Graph, ctx.Parsed)
	})
}
