// Copyright 2018-2019 Couchbase, Inc. All rights reserved.

package gojsonsm

import (
	"fmt"
	"github.com/alecthomas/participle"
	"math"
	"strings"
)

// EBNF Grammar describing the parser

// FilterExpression         = ( AndCondition { "OR" AndCondition } ) { "AND" FilterExpression }
// AndCondition             = { OpenParens } Condition { "AND" Condition } { CloseParen }
// Condition                = ( [ "NOT" ] Condition ) | Operand
// Operand                  = BooleanExpr | ( LHS ( CheckOp | ( CompareOp RHS) ) )
// BooleanExpr              = Boolean | BooleanFuncExpr
// LHS                      = ConstFuncExpr | Boolean | Field | Value
// RHS                      = ConstFuncExpr | Boolean | Value | Field
// CompareOp                = "=" | "==" | "<>" | "!=" | ">" | ">=" | "<" | "<="
// CheckOp                  = ( "IS" [ "NOT" ] ( NULL | MISSING ) )
// Field                    = { @"-" } OnePath { "." OnePath } { MathOp MathValue }
// OnePath                  = ( PathFuncExpression | StringType ){ ArrayIndex }
// StringType               = @String | @Ident | @RawString | @Char
// ArrayIndex               = "[" @Int "]"
// Value                    = @String
// ConstFuncExpr            = ConstFuncNoArg | ConstFuncOneArg | ConstFuncTwoArgs
// ConstFuncNoArg           = ConstFuncNoArgName "(" ")"
// ConstFuncNoArgName       = "PI" | "E"
// ConstFuncOneArg          = ConstFuncOneArgName "(" ConstFuncArgument ")"
// ConstFuncOneArgName      = "ABS" | "ACOS"...
// ConstFuncTwoArgs         = ConstFuncTwoArgsName "(" ConstFuncArgument "," ConstFuncArgument ")"
// ConstFuncTwoArgsName     = "ATAN2" | "POW"
// ConstFuncArgument        = Field | Value | ConstFuncExpr

// should this be   ConstFuncArgumentRHS     = Value | ConstFuncExpr
// ConstFuncArgumentRHS     = Value
// PathFuncExpression       = OnePathFuncNoArg
// OnePathFuncNoArg         = OnePathFuncNoArgName "(" ")"
// MathOp                   = @"+" | @"-" | @"*" | @"/" | @"%"
// MathValue                = @Int | @Float
// OnePathFuncNoArgName     = "META"
// BooleanFuncExpr          = BooleanFuncTwoArgs | ExistsClause
// BooleanFuncTwoArgs       = BooleanFuncTwoArgsName "(" ConstFuncArgument "," ConstFuncArgumentRHS ")"
// BooleanFuncTwoArgsName   = "REGEXP_CONTAINS"
// ExistsClause              = ( "EXISTS" "(" Field ")" )

type FilterExpression struct {
	AndConditions []*FEAndCondition   `( @@ { "OR" @@ } )`
	SubFilterExpr []*FilterExpression `{ "AND" @@ }`
}

func (f *FilterExpression) GetTotalOpenParens() (count int) {
	if len(f.AndConditions) > 0 {
		for _, ac := range f.AndConditions {
			count += ac.GetTotalOpenParens()
		}
	}
	if len(f.SubFilterExpr) > 0 {
		for _, se := range f.SubFilterExpr {
			count += se.GetTotalOpenParens()
		}
	}
	return
}

func (f *FilterExpression) GetTotalCloseParens() (count int) {
	if len(f.AndConditions) > 0 {
		for _, ac := range f.AndConditions {
			count += ac.GetTotalCloseParens()
		}
	}
	if len(f.SubFilterExpr) > 0 {
		for _, se := range f.SubFilterExpr {
			count += se.GetTotalCloseParens()
		}
	}
	return
}

func (fe *FilterExpression) String() string {
	output := []string{}

	first := true
	for _, expr := range fe.AndConditions {
		if first {
			first = false
		} else {
			output = append(output, OperatorOr)
		}
		output = append(output, expr.String())
	}

	for _, expr := range fe.SubFilterExpr {
		if first {
			first = false
		} else {
			output = append(output, OperatorAnd)
		}
		output = append(output, expr.String())
	}

	return strings.Join(output, " ")
}

// Outputs the head of the Expression match tree of which represents everything underneath
func (f *FilterExpression) OutputExpression() (Expression, error) {
	var outExpr OrExpr

	// a stricter check is to check each subexpr is paren balanced, e.g., by letting each subexpr do the check itself
	if f.GetTotalOpenParens() != f.GetTotalCloseParens() {
		return outExpr, ErrorMalformedParenthesis
	}

	for _, oneExpr := range f.AndConditions {
		andExpr, err := oneExpr.OutputExpression()
		if err != nil {
			// better return nil, err
			return outExpr, err
		}
		outExpr = append(outExpr, andExpr)
	}

	if len(f.SubFilterExpr) > 0 {
		var combinedExpr AndExpr
		combinedExpr = append(combinedExpr, outExpr)

		for _, subFilterExpr := range f.SubFilterExpr {
			subExpr, err := subFilterExpr.OutputExpression()
			if err != nil {
				// better return nil, err
				return combinedExpr, err
			}
			combinedExpr = append(combinedExpr, subExpr)
		}

		return combinedExpr, nil
	} else {
		return outExpr, nil
	}
}

type FEOpenParen struct {
	Parens string `@"("`
}

func (feop *FEOpenParen) String() string {
	return "("
}

type FECloseParen struct {
	Parens string `@")"`
}

func (fecp *FECloseParen) String() string {
	return ")"
}

type FEAndCondition struct {
	OpenParens []*FEOpenParen `{ @@ }`
	// better rename to Conditions
	OrConditions []*FECondition  `@@ { "AND" @@ }`
	CloseParens  []*FECloseParen `{ @@ }`
}

func (f *FEAndCondition) GetTotalOpenParens() (count int) {
	count += len(f.OpenParens)
	if len(f.OrConditions) > 0 {
		for _, cond := range f.OrConditions {
			count += cond.GetTotalOpenParens()
		}
	}
	return
}

func (f *FEAndCondition) GetTotalCloseParens() (count int) {
	count += len(f.CloseParens)
	if len(f.OrConditions) > 0 {
		for _, cond := range f.OrConditions {
			count += cond.GetTotalCloseParens()
		}
	}
	return
}

func (ac *FEAndCondition) String() string {
	output := []string{}

	for _, e := range ac.OpenParens {
		output = append(output, e.String())
	}

	first := true
	for _, e := range ac.OrConditions {
		if first {
			first = false
		} else {
			output = append(output, OperatorAnd)
		}
		output = append(output, e.String())
	}

	for _, e := range ac.CloseParens {
		output = append(output, e.String())
	}

	return strings.Join(output, " ")
}

func (f *FEAndCondition) OutputExpression() (Expression, error) {
	var outExpr AndExpr
	for _, oneExpr := range f.OrConditions {
		expr, err := oneExpr.OutputExpression()
		if err != nil {
			// better return nil, err
			return outExpr, err
		}
		outExpr = append(outExpr, expr)
	}
	return outExpr, nil
}

type FECondition struct {
	Not     *FECondition `"NOT" @@`
	Operand *FEOperand   `| @@`
}

func (f *FECondition) GetTotalOpenParens() (count int) {
	if f.Not != nil {
		count += f.Not.GetTotalOpenParens()
	}
	// Operand has no open or close parens
	return
}

func (f *FECondition) GetTotalCloseParens() (count int) {
	if f.Not != nil {
		count += f.Not.GetTotalCloseParens()
	}
	// Operand has no open or close parens
	return
}

func (fec *FECondition) String() string {
	// a simple string should do
	var outputStr []string

	if fec.Not != nil {
		outputStr = append(outputStr, fmt.Sprintf("%v %v", OperatorNot, fec.Not.String()))
	} else if fec.Operand != nil {
		outputStr = append(outputStr, fec.Operand.String())
	} else {
		outputStr = append(outputStr, "?? (FECondition)")
	}

	return strings.Join(outputStr, " ")
}

func (f *FECondition) OutputExpression() (Expression, error) {
	if f.Not != nil {
		subNot, err := f.Not.OutputExpression()
		return NotExpr{subNot}, err
	} else if f.Operand != nil {
		return f.Operand.OutputExpression()
	} else {
		return nil, fmt.Errorf("Invalid FECondition %v", f.String())
	}
}

type FEOperand struct {
	// not sure how the grouping on "(" works. if we have "LHS OP RHS",
	// would this produce "( @@ ( ( @@ @@ )", which is not balanced?
	BooleanExpr *FEBooleanExpr `@@ |`
	LHS         *FELhs         `( @@ (`
	Op          *FECompareOp   `( @@`
	RHS         *FERhs         `@@ ) | `
	CheckOp     *FECheckOp     `@@ ) )`
}

func (feo *FEOperand) String() string {
	if feo.BooleanExpr != nil {
		return feo.BooleanExpr.String()
	} else if feo.LHS != nil && feo.CheckOp != nil {
		return fmt.Sprintf("%v %v", feo.LHS.String(), feo.CheckOp.String())
	} else if feo.LHS != nil && feo.Op != nil && feo.RHS != nil {
		return fmt.Sprintf("%v %v %v", feo.LHS.String(), feo.Op.String(), feo.RHS.String())
	} else {
		return "?? (FEOperand)"
	}
}

func (f *FEOperand) OutputExpression() (Expression, error) {
	if f.BooleanExpr != nil {
		return f.BooleanExpr.OutputExpression()
	} else if f.LHS != nil {
		lhsExpr, err := f.LHS.OutputExpression()
		if err != nil {
			return nil, err
		}

		if f.CheckOp != nil {
			outExpr, err := f.CheckOp.OutputExpression(lhsExpr)
			// better return nil, err if err!= nil
			return outExpr, err
		} else if f.Op != nil && f.RHS != nil {
			rhsExpr, err := f.RHS.OutputExpression()
			if err != nil {
				return nil, err
			}
			return f.Op.OutputExpression(lhsExpr, rhsExpr)
		} else {
			return nil, fmt.Errorf("Invalid FEOperand %v", f.String())
		}
	} else {
		return nil, fmt.Errorf("Invalid FEOperand %v", f.String())
	}
}

type FEBooleanExpr struct {
	BooleanVal  *FEBoolean         `@@ |`
	BooleanFunc *FEBooleanFuncExpr `@@`
}

func (be *FEBooleanExpr) String() string {
	if be.BooleanVal != nil {
		return be.BooleanVal.String()
	} else if be.BooleanFunc != nil {
		return be.BooleanFunc.String()
	} else {
		return "?? (FEBooleanExpr)"
	}
}

func (f *FEBooleanExpr) OutputExpression() (Expression, error) {
	if f.BooleanVal != nil {
		return f.BooleanVal.OutputExpression(false /*asValue*/)
	} else if f.BooleanFunc != nil {
		return f.BooleanFunc.OutputExpression()
	}

	return nil, fmt.Errorf("Invalid FEBooleanExpr %v", f.String())
}

type FEBoolean struct {
	TVal  *bool `@"TRUE" |`
	TVal1 *bool `@"true" |`
	FVal  *bool `@"FALSE" |`
	FVal1 *bool `@"false"`
}

func (feb *FEBoolean) String() string {
	if feb.TVal != nil && *feb.TVal == true {
		return fmt.Sprintf("%v(bool)", OperatorTrue)
	} else if feb.TVal1 != nil && *feb.TVal1 == true {
		return fmt.Sprintf("%v(bool)", strings.ToLower(OperatorTrue))
	} else if feb.FVal != nil && *feb.FVal == true {
		return fmt.Sprintf("%v(bool)", OperatorFalse)
	} else if feb.FVal1 != nil && *feb.FVal1 == true {
		return fmt.Sprintf("%v(bool)", strings.ToLower(OperatorFalse))
	}
	// better return ?? or some other invalid val marker
	return ""
}

// can this piggyback on String() to centralize logic? (return strings.ToUpper(String()) == OperatorTrue)
// Should use IsSet() to make sure it's first set
func (feb *FEBoolean) GetBool() bool {
	if feb.TVal != nil && *feb.TVal == true {
		return true
	} else if feb.TVal1 != nil && *feb.TVal1 == true {
		return true
	} else if feb.FVal != nil && *feb.FVal == true {
		return false
	} else if feb.FVal1 != nil && *feb.FVal1 == true {
		return false
	}
	return false
}

func (feb *FEBoolean) IsSet() bool {
	return feb.TVal != nil || feb.TVal1 != nil || feb.FVal != nil || feb.FVal1 != nil
}

func (f *FEBoolean) OutputExpression(asValue bool) (Expression, error) {
	if !f.IsSet() {
		return nil, fmt.Errorf("Invalid FEBoolean (not set)")
	}
	if f.GetBool() == true {
		if asValue {
			return ValueExpr{true}, nil
		} else {
			return TrueExpr{}, nil
		}
	} else {
		if asValue {
			return ValueExpr{false}, nil
		} else {
			return FalseExpr{}, nil
		}
	}
}

type FELhs struct {
	Func  *FEConstFuncExpression `( @@ |`
	Bool  *FEBoolean             `@@ |`
	Field *FEField               `@@ |`
	Value *FEValue               `@@ )`
}

func (fel *FELhs) String() string {
	if fel.Field != nil {
		return fel.Field.String()
	} else if fel.Value != nil {
		return fel.Value.String()
	} else if fel.Func != nil {
		return fel.Func.String()
	} else if fel.Bool != nil {
		return fel.Bool.String()
	} else {
		return "?? (FELhs)"
	}
}

func (f *FELhs) OutputExpression() (Expression, error) {
	if f.Field != nil {
		return f.Field.OutputExpression()
	} else if f.Value != nil {
		return f.Value.OutputExpression()
	} else if f.Func != nil {
		return f.Func.OutputExpression()
	} else if f.Bool != nil {
		return f.Bool.OutputExpression(true /* asValue */)
	} else {
		return nil, fmt.Errorf("Invalid FELhs %v", f.String())
	}
}

// Normally users do values on the RHS, so prioritize it over field
type FERhs struct {
	Func  *FEConstFuncExpression `( @@ |`
	Bool  *FEBoolean             `@@ |`
	Value *FEValue               `@@ |`
	Field *FEField               `@@ )`
}

func (fer *FERhs) String() string {
	if fer.Field != nil {
		return fer.Field.String()
	} else if fer.Value != nil {
		return fer.Value.String()
	} else if fer.Func != nil {
		return fer.Func.String()
	} else if fer.Bool != nil {
		return fer.Bool.String()
	} else {
		return "?? (FERhs)"
	}
}

func (f *FERhs) OutputExpression() (Expression, error) {
	if f.Field != nil {
		return f.Field.OutputExpression()
	} else if f.Value != nil {
		return f.Value.OutputExpression()
	} else if f.Func != nil {
		return f.Func.OutputExpression()
	} else if f.Bool != nil {
		return f.Bool.OutputExpression(true /*asValue*/)
	} else {
		return nil, fmt.Errorf("Invalid FERhs %v", f.String())
	}
}

type FEField struct {
	MathNeg   *bool               `{ @"-" }`
	Path      []*FEOnePath        `@@ { "." @@ }`
	MathOp    *FEMathArithmeticOp `{ ( @@`
	MathValue *FEMathValue        `@@ ) }`
}

func (fef *FEField) String() string {
	output := []string{}
	outerOutput := []string{}
	for _, onePath := range fef.Path {
		output = append(output, onePath.String())
	}
	fieldOutput := strings.Join(output, ".")
	if fef.MathNeg != nil {
		fieldOutput = fmt.Sprintf("%v%v", "-", fieldOutput)
	}
	outerOutput = append(outerOutput, fieldOutput)
	if fef.MathOp != nil {
		outerOutput = append(outerOutput, fef.MathOp.String())
	}
	// validate that MathValue != nil when MathOp != nil?
	if fef.MathValue != nil {
		outerOutput = append(outerOutput, fef.MathValue.String())
	}
	return strings.Join(outerOutput, " ")
}

func (f *FEField) OutputExpression() (Expression, error) {
	var outExpr FieldExpr

	if f.ShouldHandleSpecialValue() {
		return f.OutputExpressionSpecialAsValue()
	}

	for _, onePath := range f.Path {
		pathName, arrays, err := onePath.OutputOnePath()
		if err != nil {
			// retrn nil err
			return outExpr, err
		}
		outExpr.Path = append(outExpr.Path, pathName)
		for _, arrIdx := range arrays {
			outExpr.Path = append(outExpr.Path, arrIdx)
		}
	}

	// following is a better way to structure code
	// mathOutExpr = outExpr
	// if Neg != nil {
	//   mathOutExpr =  FuncExpr{FuncName: MathFuncNeg} ...
	// }
	// if MethOp != nil {
	//   newMathoutExpr = combine mathOutExpr, Op and Value
	//    mathOutExpr = newMathOutExpr
	// }
	// return mathOutExpr
	if f.MathNeg != nil || (f.MathOp != nil && f.MathValue != nil) {
		var mathOutExpr FuncExpr
		if f.MathOp == nil {
			// Only thing is a negation of the field value
			mathOutExpr.FuncName = MathFuncNeg
			mathOutExpr.Params = append(mathOutExpr.Params, outExpr)
		} else {
			// {-}field mathOp mathVal
			mathOpExpr, err := f.MathOp.OutputExpression()
			if err != nil {
				return nil, err
			}
			mathOutExpr = mathOpExpr.(FuncExpr)

			if f.MathNeg != nil {
				negativeFieldExpr := FuncExpr{FuncName: MathFuncNeg}
				negativeFieldExpr.Params = append(negativeFieldExpr.Params, outExpr)
				mathOutExpr.Params = append(mathOutExpr.Params, negativeFieldExpr)
			} else {
				mathOutExpr.Params = append(mathOutExpr.Params, outExpr)
			}

			valueExpr, err := f.MathValue.OutputExpression()
			if err != nil {
				return nil, err
			}
			mathOutExpr.Params = append(mathOutExpr.Params, valueExpr)
		}
		return mathOutExpr, nil
	} else {
		return outExpr, nil
	}
}

// other than date value, if a literal string is incorrectly captured as FEField, would it be problematic as well?
// shouldn't literals be inside quotes like "" and ''? can't we use quotes for differentiation?

// The problem with this parser is that if FEField is prioritized higher than FEValue, then
// it's possible that a valid FEValue gets captured by FEField as it will soak up any valid @String
// or @Ident. Thus any date constant values will be put in the same category as @String and gets captured.
// This is a hack that should get around it until there's a better way to do this
// Normally, this isn't a problem because there is FERhs vs FELhs, of which FERhs will prioritize value
// over field. In the DATE() function, however, it doesn't have this luxury to prioritize value or field
// since it only has one variable.
func (f *FEField) ShouldHandleSpecialValue() bool {
	if len(f.Path) == 1 {
		if iso8601Year.MatchString(f.Path[0].String()) ||
			iso8601YearAndMonth.MatchString(f.Path[0].String()) ||
			iso8601CompleteDate.MatchString(f.Path[0].String()) {
			return true
		}
	}
	return false
}

func (f *FEField) OutputExpressionSpecialAsValue() (Expression, error) {
	return ValueExpr{f.Path[0].String()}, nil
}

type FEStringType struct {
	EscapedStrVal string `( @String  |`
	CharVal       string `@Char |`
	RawStr        string `@RawString |`
	StrValue      string `@Ident )`
}

func (f *FEStringType) String() string {
	if len(f.CharVal) > 0 {
		return f.CharVal
	} else if len(f.RawStr) > 0 {
		return f.RawStr
	} else if len(f.StrValue) > 0 {
		return f.StrValue
	} else if len(f.EscapedStrVal) > 0 {
		return f.EscapedStrVal
	} else {
		// return error symbol?
		return ""
	}
}

type FEOnePath struct {
	OnePathFunc  *FEOnePathFuncExpr `( @@  |`
	StrValue     *FEStringType      ` @@ )`
	ArrayIndexes []*FEArrayIndex    `{ @@ }`
}

func (feop *FEOnePath) String() string {
	output := []string{}
	if feop.OnePathFunc != nil {
		output = append(output, feop.OnePathFunc.String())
	} else if len(feop.StrValue.String()) > 0 {
		output = append(output, feop.StrValue.String())
	} else {
		// append error?
		output = append(output, "")
	}
	for i := 0; i < len(feop.ArrayIndexes); i++ {
		output = append(output, feop.ArrayIndexes[i].String())
	}
	return strings.Join(output, " ")
}

// Outputs a path, and an array of indexes, if there is any
func (f *FEOnePath) OutputOnePath() (string, []string, error) {
	var arrayIdx []string
	for _, arr := range f.ArrayIndexes {
		arrayIdx = append(arrayIdx, arr.String())
	}

	if f.StrValue != nil {
		return f.StrValue.String(), arrayIdx, nil
	} else if f.OnePathFunc != nil {
		return f.OnePathFunc.String(), arrayIdx, nil
	} else {
		return "", arrayIdx, fmt.Errorf("Invalid internal FEOnePath: %v", f.String())
	}
}

type FEArrayIndex struct {
	// For now we are not supporting negative indexes
	// ArrayIndex string `"[" [ @"-" ] @Int "]"`
	ArrayIndex string `"[" @Int "]"`
}

func (i *FEArrayIndex) String() string {
	return fmt.Sprintf("[%v]", i.ArrayIndex)
}

type FEOnePathFuncExpr struct {
	OnePathFuncNoArg *FEOnePathFuncNoArg `@@`
}

func (e *FEOnePathFuncExpr) String() string {
	if e.OnePathFuncNoArg != nil {
		return e.OnePathFuncNoArg.String()
	} else {
		return "?? FEOnePathFuncExpr"
	}
}

type FEOnePathFuncNoArg struct {
	OnePathFuncNoArgName *FEOnePathFuncNoArgName `( @@ "(" ")" )`
}

func (na *FEOnePathFuncNoArg) String() string {
	if na.OnePathFuncNoArgName != nil {
		return fmt.Sprintf("%v()", na.OnePathFuncNoArgName.String())
	} else {
		return "?? (FEOnePathFuncNoArg)"
	}
}

type FEOnePathFuncNoArgName struct {
	Meta *bool `@"META"`
}

func (n *FEOnePathFuncNoArgName) String() string {
	if n.Meta != nil && *n.Meta == true {
		return OperatorMeta
	} else {
		return "?? (FEOnePathFuncNoArgName)"
	}
}

// There's currently no special Expression for META function, but it's useful to have a parser gramar for it
// as it is being used internally
func (f *FEOnePathFuncNoArgName) OutputExpression() (Expression, error) {
	return nil, fmt.Errorf("Not supported (FEOnePathFuncNoArgName) %v", f.String())
}

type FEMathArithmeticOp struct {
	Addition    *bool `@"+" |`
	Subtraction *bool `@"-" |`
	Multiply    *bool `@"*" |`
	Division    *bool `@"/" |`
	Modulo      *bool `@"%"`
}

func (f *FEMathArithmeticOp) String() string {
	if f.Addition != nil {
		return "+"
	} else if f.Subtraction != nil {
		return "-"
	} else if f.Multiply != nil {
		return "*"
	} else if f.Division != nil {
		return "/"
	} else if f.Modulo != nil {
		return "%"
	} else {
		return "?? (FEMathArithmeticOp)"
	}
}

func (f *FEMathArithmeticOp) OutputExpression() (Expression, error) {
	if f.Addition != nil {
		return FuncExpr{FuncName: MathFuncAdd}, nil
	} else if f.Subtraction != nil {
		return FuncExpr{FuncName: MathFuncSub}, nil
	} else if f.Multiply != nil {
		return FuncExpr{FuncName: MathFuncMul}, nil
	} else if f.Division != nil {
		return FuncExpr{FuncName: MathFuncDiv}, nil
	} else if f.Modulo != nil {
		return FuncExpr{FuncName: MathFuncMod}, nil
	} else {
		return nil, fmt.Errorf("Invalid FEMathArithmeticOp %v", f.String())
	}
}

type FEMathValue struct {
	IntValue   *int     `@Int |`
	FloatValue *float64 `@Float`
}

func (f *FEMathValue) String() string {
	if f.IntValue != nil {
		return fmt.Sprintf("%v", *f.IntValue)
	} else if f.FloatValue != nil {
		return fmt.Sprintf("%v", *f.FloatValue)
	} else {
		return "?? (FEMathValue)"
	}
}

func (f *FEMathValue) OutputExpression() (Expression, error) {
	if f.IntValue != nil {
		return ValueExpr{*f.IntValue}, nil
	} else if f.FloatValue != nil {
		return ValueExpr{*f.FloatValue}, nil
	} else {
		return nil, fmt.Errorf("Invalid FEMathValue %v", f.String())
	}
}

type FEValue struct {
	StrValue   *string  `@String |`
	IntValue   *int     `@Int |`
	FloatValue *float64 `@Float`
}

func (fev *FEValue) String() string {
	if fev.StrValue != nil {
		return *fev.StrValue
	} else if fev.IntValue != nil {
		return fmt.Sprintf("%v", *fev.IntValue)
	} else if fev.FloatValue != nil {
		return fmt.Sprintf("%v", *fev.FloatValue)
	} else {
		return "?? (FEValue)"
	}
}

func (f *FEValue) OutputExpression() (Expression, error) {
	if f.StrValue != nil {
		return ValueExpr{
			*f.StrValue,
		}, nil
	} else if f.IntValue != nil {
		return ValueExpr{
			*f.IntValue,
		}, nil
	} else if f.FloatValue != nil {
		return ValueExpr{
			*f.FloatValue,
		}, nil
	} else {
		return ValueExpr{}, fmt.Errorf("Invalid FEValue: %v", f.String())
	}
}

// i do not get the complication. is it that op chars could collide with symbols in other types of operands?
// even so doing multiple-char matching, e.g., trying to match "<>", should reduce the chance of colliding.

// We have to do this funky way of matching because our FEOperand expression may not be composed of a compareOp
// And due to the complicated FEOperand op, we have to do char by char match so we can catch the not-matched case
// and go to the other type of operands

type FEOpChar struct {
	Not         *bool `( @"!" |`
	Equal       *bool `@"=" |`
	LessThan    *bool `@"<" |`
	GreaterThan *bool `@">" )`
}

func (f *FEOpChar) String() string {
	if f.Not != nil {
		return "!"
	} else if f.Equal != nil {
		return "="
	} else if f.LessThan != nil {
		return "<"
	} else if f.GreaterThan != nil {
		return ">"
	}
	return ""
}

type FECompareOp struct {
	OpChars0 *FEOpChar `@@`
	OpChars1 *FEOpChar `[ @@ ]`
}

func (feo *FECompareOp) IsEqual() bool {
	// =
	singleEq := feo.OpChars0 != nil && feo.OpChars0.Equal != nil && feo.OpChars1 == nil
	// ==
	doubleEq := feo.OpChars0 != nil && feo.OpChars0.Equal != nil && feo.OpChars1 != nil && feo.OpChars1.Equal != nil
	return singleEq || doubleEq
}

func (feo *FECompareOp) IsNotEqual() bool {
	// !=
	notEqual0 := feo.OpChars0 != nil && feo.OpChars0.Not != nil && feo.OpChars1 != nil && feo.OpChars1.Equal != nil
	// <>
	notEqual1 := feo.OpChars0 != nil && feo.OpChars0.LessThan != nil && feo.OpChars1 != nil && feo.OpChars1.GreaterThan != nil
	return notEqual0 || notEqual1
}

func (feo *FECompareOp) IsGreaterThan() bool {
	// >
	return feo.OpChars0 != nil && feo.OpChars0.GreaterThan != nil && feo.OpChars1 == nil
}

func (feo *FECompareOp) IsGreaterThanOrEqualTo() bool {
	// >=
	return feo.OpChars0 != nil && feo.OpChars0.GreaterThan != nil && feo.OpChars1 != nil && feo.OpChars1.Equal != nil
}

func (feo *FECompareOp) IsLessThan() bool {
	// <
	return feo.OpChars0 != nil && feo.OpChars0.LessThan != nil && feo.OpChars1 == nil
}

func (feo *FECompareOp) IsLessThanOrEqualTo() bool {
	// <=
	return feo.OpChars0 != nil && feo.OpChars0.LessThan != nil && feo.OpChars1 != nil && feo.OpChars1.Equal != nil
}

func (feo *FECompareOp) String() string {
	if feo.IsEqual() {
		return OperatorEquals
	} else if feo.IsNotEqual() {
		return OperatorNotEquals
	} else if feo.IsGreaterThan() {
		return OperatorGreaterThan
	} else if feo.IsGreaterThanOrEqualTo() {
		return OperatorGreaterThanEq
	} else if feo.IsLessThan() {
		return OperatorLessThan
	} else if feo.IsLessThanOrEqualTo() {
		return OperatorLessThanEq
	}
	var invalidOp []string
	if feo.OpChars0 != nil {
		invalidOp = append(invalidOp, feo.OpChars0.String())
	}
	if feo.OpChars1 != nil {
		invalidOp = append(invalidOp, feo.OpChars1.String())
	}
	if len(invalidOp) > 0 {
		return strings.Join(invalidOp, "")
	} else {
		return "?? (FECompareOp)"
	}
}

func (f *FECompareOp) OutputExpression(lhs Expression, rhs Expression) (Expression, error) {
	if f.IsEqual() {
		return EqualsExpr{
			Lhs: lhs,
			Rhs: rhs,
		}, nil
	} else if f.IsNotEqual() {
		return NotEqualsExpr{
			Lhs: lhs,
			Rhs: rhs,
		}, nil
	} else if f.IsGreaterThan() {
		return GreaterThanExpr{
			Lhs: lhs,
			Rhs: rhs,
		}, nil
	} else if f.IsGreaterThanOrEqualTo() {
		return GreaterEqualsExpr{
			Lhs: lhs,
			Rhs: rhs,
		}, nil
	} else if f.IsLessThan() {
		return LessThanExpr{
			Lhs: lhs,
			Rhs: rhs,
		}, nil
	} else if f.IsLessThanOrEqualTo() {
		return LessEqualsExpr{
			Lhs: lhs,
			Rhs: rhs,
		}, nil
	}
	return nil, fmt.Errorf("Invalid FECompareOp %v", f.String())
}

type FECheckOp struct {
	Not     *bool `( "IS" [ @"NOT" ]`
	Null    *bool `( @"NULL" |`
	Missing *bool `@"MISSING" ) )`
}

func (feco *FECheckOp) isNot() bool {
	return feco.Not != nil && *feco.Not == true
}

func (feco *FECheckOp) IsMissing() bool {
	return !feco.isNot() && feco.isMissingInternal()
}

func (feco *FECheckOp) isMissingInternal() bool {
	return feco.Missing != nil && *feco.Missing == true
}

func (feco *FECheckOp) IsNotMissing() bool {
	return feco.isNot() && feco.isMissingInternal()
}

func (feco *FECheckOp) IsNull() bool {
	return !feco.isNot() && feco.isNullInternal()
}

func (feco *FECheckOp) isNullInternal() bool {
	return feco.Null != nil && *feco.Null == true
}

func (feco *FECheckOp) IsNotNull() bool {
	return feco.isNot() && feco.isNullInternal()
}

func (feco *FECheckOp) String() string {
	if feco.IsMissing() {
		return OperatorMissing
	} else if feco.IsNotMissing() {
		return OperatorNotMissing
	} else if feco.IsNull() {
		return OperatorNull
	} else if feco.IsNotNull() {
		return OperatorNotNull
	} else {
		return "?? (FECheckOp)"
	}
}

func (f *FECheckOp) OutputExpression(subExpr Expression) (Expression, error) {
	if f.IsNotMissing() {
		return ExistsExpr{
			subExpr,
		}, nil
	} else if f.IsMissing() {
		return NotExistsExpr{
			subExpr,
		}, nil
	} else if f.IsNull() {
		return EqualsExpr{
			subExpr,
			ValueExpr{nil},
		}, nil
	} else if f.IsNotNull() {
		return NotExpr{
			EqualsExpr{
				subExpr,
				ValueExpr{nil},
			},
		}, nil
	}

	return nil, fmt.Errorf("Invalid FECheckOp %v", f.String())
}

// Technically we could have an slice of arguments, but having OneArg vs NoArg vs TwoArg could
// allow us to do more strict function check (i.e. certain funcs should only allow one argument, etc, at this level)
type FEConstFuncExpression struct {
	ConstFuncNoArg   *FEConstFuncNoArg   `@@ |`
	ConstFuncOneArg  *FEConstFuncOneArg  `@@ |`
	ConstFuncTwoArgs *FEConstFuncTwoArgs `@@`
}

func (f *FEConstFuncExpression) String() string {
	if f.ConstFuncNoArg != nil {
		return f.ConstFuncNoArg.String()
	} else if f.ConstFuncOneArg != nil {
		return f.ConstFuncOneArg.String()
	} else if f.ConstFuncTwoArgs != nil {
		return f.ConstFuncTwoArgs.String()
	} else {
		return "?? (FEConstFuncExpression)"
	}
}

func (f *FEConstFuncExpression) OutputExpression() (Expression, error) {
	if f.ConstFuncNoArg != nil {
		return f.ConstFuncNoArg.OutputExpression()
	} else if f.ConstFuncOneArg != nil {
		return f.ConstFuncOneArg.OutputExpression()
	} else if f.ConstFuncTwoArgs != nil {
		return f.ConstFuncTwoArgs.OutputExpression()
	} else {
		return nil, fmt.Errorf("Invalid FEConstFuncExpression %v", f.String())
	}
}

type FEConstFuncNoArg struct {
	ConstFuncNoArgName *FEConstFuncNoArgName `( @@ "(" ")" )`
}

func (f *FEConstFuncNoArg) String() string {
	if f.ConstFuncNoArgName != nil {
		return fmt.Sprintf("%v()", f.ConstFuncNoArgName.String())
	} else {
		return "?? (FEConstFuncNoArg)"
	}
}

func (f *FEConstFuncNoArg) OutputExpression() (Expression, error) {
	if f.ConstFuncNoArgName == nil {
		return nil, fmt.Errorf("Invalid FEConstFuncNoArg")
	} else if f.ConstFuncNoArgName.Pi != nil && *f.ConstFuncNoArgName.Pi {
		return ValueExpr{float64(math.Pi)}, nil
	} else if f.ConstFuncNoArgName.E != nil && *f.ConstFuncNoArgName.E {
		return ValueExpr{float64(math.E)}, nil
	} else {
		return nil, fmt.Errorf("Invalid FEConstFuncNoArg")
	}
}

type FEConstFuncNoArgName struct {
	Pi *bool `@"PI" |` // FuncPi
	E  *bool `@"E"`    // FuncE
}

func (n *FEConstFuncNoArgName) String() string {
	if n.E != nil && *n.E == true {
		return "E"
	} else if n.Pi != nil && *n.Pi == true {
		return "PI"
	} else {
		return "?? (FEConstFuncNoArgName)"
	}
}

// the comment "Prioritize value over field" seems to belong here

// Order matters
type FEConstFuncArgument struct {
	SubFunc  *FEConstFuncExpression `@@ |`
	Field    *FEField               `@@ |`
	Argument *FEValue               `@@`
}

func (arg *FEConstFuncArgument) String() string {
	if arg.Argument != nil {
		return arg.Argument.String()
	} else if arg.SubFunc != nil {
		return arg.SubFunc.String()
	} else if arg.Field != nil {
		return arg.Field.String()
	} else {
		return "?? (FEConstFuncArgument)"
	}
}

func (f *FEConstFuncArgument) OutputExpression() (Expression, error) {
	if f.Argument != nil {
		return f.Argument.OutputExpression()
	} else if f.Field != nil {
		return f.Field.OutputExpression()
	} else if f.SubFunc != nil {
		return f.SubFunc.OutputExpression()
	} else {
		return nil, fmt.Errorf("Invalid FEConstFuncArgument %v", f.String())
	}
}

// comment not applicable
// Prioritize value over field
type FEConstFuncArgumentRHS struct {
	SubFunc  *FEConstFuncExpression `@@ |`
	Argument *FEValue               `@@`
}

func (arg *FEConstFuncArgumentRHS) String() string {
	if arg.Argument != nil {
		return arg.Argument.String()
	} else if arg.SubFunc != nil {
		return arg.SubFunc.String()
	} else {
		return "?? (FEConstFuncArgument)"
	}
}

func (f *FEConstFuncArgumentRHS) OutputExpression() (Expression, error) {
	if f.SubFunc != nil {
		return f.SubFunc.OutputExpression()
	} else if f.Argument != nil {
		return f.Argument.OutputExpression()
	} else {
		return nil, fmt.Errorf("Invalid FEConstFuncArgumentRHS %v", f.String())
	}
}

func (f *FEConstFuncArgumentRHS) OutputRegexExpression() (Expression, error) {
	if f.Argument == nil {
		return nil, fmt.Errorf("Invalid FEConstFuncArgumentRHS for regex expression %v", f.String())
	}
	if tokenIsPcreValueType(f.Argument.String()) {
		return MakePcreExpression(f.Argument.String())
	} else {
		return RegexExpr{f.Argument.String()}, nil
	}
}

type FEConstFuncOneArg struct {
	ConstFuncOneArgName *FEConstFuncOneArgName `( @@ "("`
	Argument            *FEConstFuncArgument   `@@ ")" )`
}

func (oa *FEConstFuncOneArg) String() string {
	if oa.ConstFuncOneArgName == nil || oa.Argument == nil {
		return "?? (FEConstFuncOneArg)"
	}
	return fmt.Sprintf("%v( %v )", oa.ConstFuncOneArgName.String(), oa.Argument.String())
}

func (f *FEConstFuncOneArg) OutputExpression() (Expression, error) {
	var outExpr FuncExpr
	if f.ConstFuncOneArgName == nil || f.Argument == nil {
		return outExpr, fmt.Errorf("Invalid FEConstFuncOneArg %v", f.String())
	}
	name, err := f.ConstFuncOneArgName.OutputExpression()
	if err != nil {
		// nil, err
		return outExpr, err
	}
	outExpr.FuncName = name
	arg, err := f.Argument.OutputExpression()
	if err != nil {
		// nil, err
		return outExpr, err
	}
	outExpr.Params = append(outExpr.Params, arg)
	return outExpr, nil
}

type FEConstFuncOneArgName struct {
	// N1QL also supports sign(expr) and random(expr)
	Abs     *bool `@"ABS" |`
	Acos    *bool `@"ACOS" |`
	Asin    *bool `@"ASIN" |`
	Atan    *bool `@"ATAN" |`
	Ceil    *bool `@"CEIL" |`
	Cos     *bool `@"COS" |`
	Date    *bool `@"DATE" |`
	Degrees *bool `@"DEGREES" |`
	Exp     *bool `@"EXP" |`
	Floor   *bool `@"FLOOR" |`
	Log     *bool `@"LOG" |`
	Ln      *bool `@"LN" |`
	Sine    *bool `@"SIN" |`
	Tangent *bool `@"TAN" |`
	Radians *bool `@"RADIANS" |`
	Round   *bool `@"ROUND" |`
	Sqrt    *bool `@"SQRT"`
}

func (arg *FEConstFuncOneArgName) String() string {
	if arg.Abs != nil && *arg.Abs == true {
		return FuncAbs
	} else if arg.Acos != nil && *arg.Acos == true {
		return FuncAcos
	} else if arg.Asin != nil && *arg.Asin == true {
		return FuncAsin
	} else if arg.Atan != nil && *arg.Atan == true {
		return FuncAtan
	} else if arg.Ceil != nil && *arg.Ceil == true {
		return FuncCeil
	} else if arg.Cos != nil && *arg.Cos == true {
		return FuncCos
	} else if arg.Date != nil && *arg.Date == true {
		return FuncDate
	} else if arg.Degrees != nil && *arg.Degrees == true {
		return FuncDeg
	} else if arg.Exp != nil && *arg.Exp == true {
		return FuncExp
	} else if arg.Floor != nil && *arg.Floor == true {
		return FuncFloor
	} else if arg.Log != nil && *arg.Log == true {
		return FuncLog
	} else if arg.Ln != nil && *arg.Ln == true {
		return FuncLn
	} else if arg.Sine != nil && *arg.Sine == true {
		return FuncSin
	} else if arg.Tangent != nil && *arg.Tangent == true {
		return FuncTan
	} else if arg.Radians != nil && *arg.Radians == true {
		return FuncRad
	} else if arg.Round != nil && *arg.Round == true {
		return FuncRound
	} else if arg.Sqrt != nil && *arg.Sqrt == true {
		return FuncSqrt
	} else {
		return "?? (FEConstFuncOneArgName)"
	}
}

func (arg *FEConstFuncOneArgName) OutputExpression() (string, error) {
	if arg.Abs != nil && *arg.Abs == true {
		return MathFuncAbs, nil
	} else if arg.Acos != nil && *arg.Acos == true {
		return MathFuncAcos, nil
	} else if arg.Asin != nil && *arg.Asin == true {
		return MathFuncAsin, nil
	} else if arg.Atan != nil && *arg.Atan == true {
		return MathFuncAtan, nil
	} else if arg.Ceil != nil && *arg.Ceil == true {
		return MathFuncCeil, nil
	} else if arg.Cos != nil && *arg.Cos == true {
		return MathFuncCos, nil
	} else if arg.Date != nil && *arg.Date == true {
		return DateFunc, nil
	} else if arg.Degrees != nil && *arg.Degrees == true {
		return MathFuncDegrees, nil
	} else if arg.Exp != nil && *arg.Exp == true {
		return MathFuncExp, nil
	} else if arg.Floor != nil && *arg.Floor == true {
		return MathFuncFloor, nil
	} else if arg.Log != nil && *arg.Log == true {
		return MathFuncLog, nil
	} else if arg.Ln != nil && *arg.Ln == true {
		return MathFuncLn, nil
	} else if arg.Sine != nil && *arg.Sine == true {
		return MathFuncSin, nil
	} else if arg.Tangent != nil && *arg.Tangent == true {
		return MathFuncTan, nil
	} else if arg.Radians != nil && *arg.Radians == true {
		return MathFuncRadians, nil
	} else if arg.Round != nil && *arg.Round == true {
		return MathFuncRound, nil
	} else if arg.Sqrt != nil && *arg.Sqrt == true {
		return MathFuncSqrt, nil
	} else {
		return "?? (FEConstFuncOneArgName)", ErrorNotFound
	}
}

type FEConstFuncTwoArgs struct {
	ConstFuncTwoArgsName *FEConstFuncTwoArgsName `( @@ "("`
	Argument0            *FEConstFuncArgument    `@@ "," `
	Argument1            *FEConstFuncArgument    `@@ ")" )`
}

func (fta *FEConstFuncTwoArgs) String() string {
	if fta.ConstFuncTwoArgsName == nil || fta.Argument0 == nil || fta.Argument1 == nil {
		return "?? (FEConstFuncTwoArgs)"
	}
	return fmt.Sprintf("%v( %v , %v )", fta.ConstFuncTwoArgsName.String(), fta.Argument0.String(), fta.Argument1.String())
}

func (f *FEConstFuncTwoArgs) OutputExpression() (Expression, error) {
	var outExpr FuncExpr
	if f.ConstFuncTwoArgsName == nil || f.Argument0 == nil || f.Argument1 == nil {
		return outExpr, fmt.Errorf("Invalid FEConstFuncTwoArgs %v", f.String())
	}
	name, err := f.ConstFuncTwoArgsName.OutputExpression()
	if err != nil {
		// nil, err
		return outExpr, err
	}
	outExpr.FuncName = name
	arg0, err := f.Argument0.OutputExpression()
	if err != nil {
		// nil, err
		return outExpr, err
	}
	arg1, err := f.Argument1.OutputExpression()
	if err != nil {
		// nil, err
		return outExpr, err
	}
	outExpr.Params = append(outExpr.Params, arg0)
	outExpr.Params = append(outExpr.Params, arg1)
	return outExpr, nil
}

type FEConstFuncTwoArgsName struct {
	// n1ql has POWER(), not POW()
	// n1ql also has ROUND() and TRUNC() which could take 1-2 args
	Atan2 *bool `@"ATAN2" |`
	Power *bool `@"POW"`
}

func (arg *FEConstFuncTwoArgsName) String() string {
	if arg.Atan2 != nil && *arg.Atan2 == true {
		return FuncAtan2
	} else if arg.Power != nil && *arg.Power == true {
		return FuncPower
	} else {
		return "?? (FEConstFuncTwoArgsName)"
	}
}

func (arg *FEConstFuncTwoArgsName) OutputExpression() (string, error) {
	if arg.Atan2 != nil && *arg.Atan2 == true {
		return MathFuncAtan2, nil
	} else if arg.Power != nil && *arg.Power == true {
		return MathFuncPow, nil
	} else {
		return "?? (FEConstFuncTwoArgsName)", ErrorNotFound
	}
}

type FEBooleanFuncExpr struct {
	BooleanFuncTwoArgs *FEBooleanFuncTwoArgs `@@ |`
	ExistsClause       *FEExistsClause       `@@`
}

func (f *FEBooleanFuncExpr) String() string {
	if f.BooleanFuncTwoArgs != nil {
		return f.BooleanFuncTwoArgs.String()
	} else if f.ExistsClause != nil {
		return f.ExistsClause.String()
	} else {
		return "?? (FEBooleanFuncExpr)"
	}
}

func (f *FEBooleanFuncExpr) OutputExpression() (Expression, error) {
	if f.BooleanFuncTwoArgs != nil {
		return f.BooleanFuncTwoArgs.OutputExpression()
	} else if f.ExistsClause != nil {
		return f.ExistsClause.OutputExpression()
	}
	return nil, fmt.Errorf("Invalid FEBooleanFuncExpr")
}

type FEBooleanFuncTwoArgs struct {
	BooleanFuncTwoArgsName *FEBooleanFuncTwoArgsName `( @@ "("`
	Argument0              *FEConstFuncArgument      `@@ ","`
	Argument1              *FEConstFuncArgumentRHS   `@@ ")" )`
}

func (a *FEBooleanFuncTwoArgs) String() string {
	if a.BooleanFuncTwoArgsName == nil || a.Argument0 == nil || a.Argument1 == nil {
		return "?? (FEBooleanFuncTwoArgs)"
	} else {
		return fmt.Sprintf("%v( %v , %v )", a.BooleanFuncTwoArgsName.String(), a.Argument0.String(), a.Argument1.String())
	}
}

func (f *FEBooleanFuncTwoArgs) OutputExpression() (Expression, error) {
	if f.BooleanFuncTwoArgsName != nil && f.BooleanFuncTwoArgsName.RegexContains != nil && *f.BooleanFuncTwoArgsName.RegexContains &&
		f.Argument0 != nil && f.Argument1 != nil {
		outputExpr, err := f.BooleanFuncTwoArgsName.OutputExpression()
		if err != nil {
			return nil, err
		}
		outExpr := outputExpr.(LikeExpr)

		arg0, err := f.Argument0.OutputExpression()
		if err != nil {
			// nil, err
			return outExpr, err
		}
		outExpr.Lhs = arg0

		arg1, err := f.Argument1.OutputRegexExpression()
		if err != nil {
			// nil, err
			return outExpr, err
		}
		outExpr.Rhs = arg1

		return outExpr, nil
	} else {
		return nil, fmt.Errorf("Invalid FEBooleanFuncTwoArgs %v", f.BooleanFuncTwoArgsName.String())
	}
}

type FEBooleanFuncTwoArgsName struct {
	RegexContains *bool `@"REGEXP_CONTAINS"`
}

func (n *FEBooleanFuncTwoArgsName) String() string {
	if n.RegexContains != nil && *n.RegexContains == true {
		return FuncRegexp
	} else {
		return "?? (FEBooleanFuncTwoArgsName)"
	}
}

func (n *FEBooleanFuncTwoArgsName) OutputExpression() (Expression, error) {
	if n.RegexContains != nil && *n.RegexContains == true {
		return LikeExpr{}, nil
	} else {
		return nil, ErrorNotFound
	}
}

type FEExistsClause struct {
	Field *FEField `( "EXISTS" "(" @@ ")" )`
}

func (f *FEExistsClause) String() string {
	if f.Field != nil {
		return fmt.Sprintf("%v ( %v )", OperatorExists, f.Field.String())
	} else {
		return "?? (FEExistsClause)"
	}
}

func (f *FEExistsClause) OutputExpression() (Expression, error) {
	if f.Field != nil {
		fieldExpr, err := f.Field.OutputExpression()
		if err != nil {
			return nil, err
		}
		return ExistsExpr{
			fieldExpr,
		}, nil
	}

	return nil, fmt.Errorf("Invalid FEExistsClause %v", f.String())
}

func parserWrapper(parser *participle.Parser, expression string, fe *FilterExpression, err *error) {
	defer func() {
		if r := recover(); r != nil {
			*err = fmt.Errorf("Error from parser: %v", r)
		}
	}()

	*err = parser.ParseString(expression, fe)
}

func NewFilterExpressionParser(expression string) (*participle.Parser, *FilterExpression, error) {
	fe := &FilterExpression{}
	if len(expression) == 0 {
		return nil, fe, ErrorEmptyInput
	}

	parser, err := participle.Build(fe)
	if err != nil {
		// nil nil err
		return parser, fe, err
	}

	// Use a wrapper so we can recover any panic and set the error gracefully
	parserWrapper(parser, expression, fe, &err)

	// return nil nil when err != nil
	return parser, fe, err
}

func GetFilterExpressionMatcher(expression string) (Matcher, error) {
	_, fe, err := NewFilterExpressionParser(expression)
	if err != nil {
		return nil, err
	}

	expr, err := fe.OutputExpression()
	if err != nil {
		return nil, err
	}

	var trans Transformer
	matchDef := trans.Transform([]Expression{expr})

	matcher := NewFastMatcher(matchDef)
	return matcher, nil
}
