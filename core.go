package ruler

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"math"
)


// validate the formatLogic string
func validLogic(logic string) (string, error) {

	formatLogic := formatLogicExpression(logic)
	if formatLogic == Space || formatLogic == EmptyStr {
		return EmptyStr, nil
	}

	// 1. `formatLogic` must be composed of legal symbol such as operator, number and bracket.
	isValidSymbol := isFormatLogicExpressionAllValidSymbol(formatLogic)
	if !isValidSymbol {
		return EmptyStr, errors.New("invalid logic expression: invalid symbol")
	}

	// 2. check `logic expression`(逻辑表达式) by trying to calculate result with random bool value
	err := tryToCalculateResultByFormatLogicExpressionWithRandomProbe(formatLogic)
	if err != nil {
		return EmptyStr, errors.New("invalid logic expression: can not calculate")
	}

	return formatLogic, nil
}

func injectLogic(rules *Rules, logic string) (*Rules, error) {
	// validate the `logic` string
	formatLogic, err := validLogic(logic)
	if err != nil {
		return nil, err
	}
	// empty `logic` means nothing
	if formatLogic == EmptyStr {
		return rules, nil
	}
	// all ids in `formatLogic` string must be in rules ids
	isValidIds := isFormatLogicExpressionAllIdsExist(formatLogic, rules)
	if !isValidIds {
		return nil, errors.New("invalid logic expression: invalid id")
	}
	// set rules.Logic with value `formatLogic`
	rules.Logic = formatLogic
	return rules, nil
}

func injectExtractInfo(rules *Rules, extractInfo map[string]string) *Rules {
	if name, ok := extractInfo["name"]; ok {
		rules.Name = name
	}
	if msg, ok := extractInfo["msg"]; ok {
		rules.Msg = msg
	}
	return rules
}


func newRulesWithJSON(jsonStr []byte) (*Rules, error) {
	var rules []*Rule
	err := json.Unmarshal(jsonStr, &rules)
	if err != nil {
		return nil, err
	}
	return newRulesWithArray(rules), nil
}

func newRulesWithArray(rules []*Rule) *Rules {
	// give rule an id
	var maxID = 1
	for _, rule := range rules {
		if rule.ID > maxID {
			maxID = rule.ID
		}
	}
	for index := range rules {
		if rules[index].ID == 0 {
			maxID++
			rules[index].ID = maxID
		}
	}
	return &Rules{
		Rules: rules,
	}
}




func (rs *Rules) fitWithMapInFact(o map[string]interface{}) (bool, map[int]string, map[int]interface{}) {
	var results = make(map[int]bool)
	var tips = make(map[int]string)
	var values = make(map[int]interface{})
	var allRuleIDs []int
	var hasLogic = false



	 
	if rs.Logic != EmptyStr {
		hasLogic = true
	}

	// 
	for _, rule := range rs.Rules {


		v := pluck(rule.Key, o)
		if v != nil && rule.Val != nil {
			typeV := reflect.TypeOf(v)
			typeR := reflect.TypeOf(rule.Val)
			if !typeV.Comparable() || !typeR.Comparable() {
				return false, nil, nil
			}
		}

		values[rule.ID] = v

		flag := rule.fit(v)
		results[rule.ID] = flag
		if !flag {
			// fit false, record msg, for no logic expression usage
			tips[rule.ID] = rule.Msg
		}
		allRuleIDs = append(allRuleIDs, rule.ID)
	}


	// compute result by considering logic
	if !hasLogic {
		for _, flag := range results {
			if !flag {
				return false, tips, values
			}
		}
		return true, rs.getTipsByRuleIDs(allRuleIDs), values
	}
	answer, ruleIDs, err := rs.calculateExpressionByTree(results)


	// tree can return fail reasons in fact
	tips = rs.getTipsByRuleIDs(ruleIDs)
	if err != nil {
		return false, nil, values
	}
	return answer, tips, values
}





func (rs *Rules) getTipsByRuleIDs(ids []int) map[int]string {
	var tips = make(map[int]string)
	var allTips = make(map[int]string)
	for _, rule := range rs.Rules {
		allTips[rule.ID] = rule.Msg
	}
	for _, id := range ids {
		tips[id] = allTips[id]
	}
	return tips
}

func (r *Rule) fit(v interface{}) bool {
	op := r.Op
	// judge if need convert to uniform type
	var ok bool
	// index-0 actual, index-1 expect
	var pairStr = make([]string, 2)
	var pairNum = make([]float64, 2)
	var isStr, isNum, isObjStr, isRuleStr bool
	pairStr[0], ok = v.(string)
	if !ok {
		pairNum[0] = formatNumber(v)
		isStr = false
		isNum = true
		isObjStr = false
	} else {
		isStr = true
		isNum = false
		isObjStr = true
	}
	pairStr[1], ok = r.Val.(string)
	if !ok {
		pairNum[1] = formatNumber(r.Val)
		isStr = false
		isRuleStr = false
	} else {
		isNum = false
		isRuleStr = true
	}

	var flagOpIn bool
	// if in || nin
	if op == "@" || op == "in" || op == "!@" || op == "nin" || op == "<<" || op == "between" {
		flagOpIn = true
		if !isObjStr && isRuleStr {
			pairStr[0] = strconv.FormatFloat(pairNum[0], 'f', -1, 64)
		}
	}

	// if types different, ignore in & nin
	if !isStr && !isNum && !flagOpIn {
		return false
	}

	switch op {
	case "=", "eq":
		if isNum {
			return pairNum[0] == pairNum[1]
		}
		if isStr {
			return pairStr[0] == pairStr[1]
		}
		return false
	case ">", "gt":
		if isNum {
			return pairNum[0] > pairNum[1]
		}
		if isStr {
			return pairStr[0] > pairStr[1]
		}
		return false
	case "<", "lt":
		if isNum {
			return pairNum[0] < pairNum[1]
		}
		if isStr {
			return pairStr[0] < pairStr[1]
		}
		return false
	case ">=", "gte":
		if isNum {
			return pairNum[0] >= pairNum[1]
		}
		if isStr {
			return pairStr[0] >= pairStr[1]
		}
		return false
	case "<=", "lte":
		if isNum {
			return pairNum[0] <= pairNum[1]
		}
		if isStr {
			return pairStr[0] <= pairStr[1]
		}
		return false
	case "!=", "neq":
		if isNum {
			return pairNum[0] != pairNum[1]
		}
		if isStr {
			return pairStr[0] != pairStr[1]
		}
		return false
	case "@", "in":
		return isIn(pairStr[0], pairStr[1], !isObjStr)
	case "!@", "nin":
		return !isIn(pairStr[0], pairStr[1], !isObjStr)
	case "^$", "regex":
		return checkRegex(pairStr[1], pairStr[0])
	case "0", "empty":
		return v == nil
	case "1", "nempty":
		return v != nil
	case "<<", "between":
		return isBetween(pairNum[0], pairStr[1])
	case "@@", "intersect":
		return isIntersect(pairStr[1], pairStr[0])
	default:
		return false
	}
}



func pluck(key string, o map[string]interface{}) interface{} {
	if o == nil || key == EmptyStr {
		return nil
	}

	paths := strings.Split(key, ".")
	var ok bool
	for index, step := range paths {
		// last step is object key
		if index == len(paths)-1 {   //返回最后一层map保存的interface{}
			return o[step]
		}

		// explore deeper
		if o, ok = o[step].(map[string]interface{}); !ok { //递归探测，失败返回nil
			return nil
		}
	}
	return nil
}


// 数值类型转换成 float64
func formatNumber(v interface{}) float64 {
	switch t := v.(type) {
	case uint:
		return float64(t)
	case uint8:
		return float64(t)
	case uint16:
		return float64(t)
	case uint32:
		return float64(t)
	case uint64:
		return float64(t)
	case int:
		return float64(t)
	case int8:
		return float64(t)
	case int16:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	default:
		return 0
	}
}



// 检查字符串 o 是否满足正则表达式 pattern
func checkRegex(pattern, o string) bool {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return regex.MatchString(o)
}



func formatLogicExpression(strRawExpr string) string {
	
	strBracket 	:= "bracket"
	strSpace 	:= "space"
	strNotSpace := "notSpace"
	
	// add space
	var flagPre, flagNow string
	runesPretty := make([]rune, 0)
	strOrigin := strings.ToLower(strRawExpr)
	for _, c := range strOrigin {
		
		// get current flag
		if c <= '9' && c >= '0' {
			flagNow = "num"
		} else if c <= 'z' && c >= 'a' {
			flagNow = "char"
		} else if c == '(' || c == ')' {
			flagNow = strBracket
		} else {
			//...
			flagNow = flagPre
		}

		// 当发生类型的变化，或者遇到了括号，就添加空格来分隔开。
		// should insert space here, in order to separate symbols by space.
		if flagNow != flagPre || flagNow == strBracket && flagPre == strBracket {
			runesPretty = append(runesPretty, []rune(Space)[0])
		}

		// append current character 
		runesPretty = append(runesPretty, c)
		flagPre = flagNow
	}

	// remove redundant space
	flagPre = strNotSpace
	runesTrim := make([]rune, 0)
	for _, c := range runesPretty {
		if c == []rune(Space)[0] {
			flagNow = strSpace
		} else {
			flagNow = strNotSpace
		}
		if flagNow == strSpace && flagPre == strSpace {
			// continuous space only save the first one
			continue
		} else {
			runesTrim = append(runesTrim, c)
		}
		flagPre = flagNow
	}
	strPrettyTrim := string(runesTrim)
	strPrettyTrim = strings.Trim(strPrettyTrim, Space)

	return strPrettyTrim
}





// 检查 strFormatLogic 中所含的是否都是合法符号，也即必须是操作符、数字、括号。
func isFormatLogicExpressionAllValidSymbol(strFormatLogic string) bool {

	regex, err := regexp.Compile(PatternNumber)
	if err != nil {
		return false
	}

	listSymbol := strings.Split(strFormatLogic, Space)
	for _, symbol := range listSymbol {
		flag := false
		// symbol is number, ignore
		if regex.MatchString(symbol) {
			continue
		}

		// symbol is operator, ok
		for _, op := range ValidOperators {
			if op == symbol {
				flag = true
			}
		}

		// symbol is bracket, ok
		for _, v := range []string{"(", ")"} {
			if v == symbol {
				flag = true
			}
		}

		// symbol is not digit, operator nor bracket, return false
		if !flag {
			return false
		}
	}
	return true
}


func isFormatLogicExpressionAllIdsExist(strFormatLogic string, rules *Rules) bool {
	mapExistIds := make(map[string]bool)

	//遍历子规则，将其 ID 添加到 map 中
	for _, eachRule := range rules.Rules {
		mapExistIds[strconv.Itoa(eachRule.ID)] = true
	}

	//纯数字的正则表达式
	regex, err := regexp.Compile(PatternNumber)
	if err != nil {
		return false
	}

	//把 strFormatLogic 按空格切分成子字符串、转换成整数、判断是否存在于 map 中，如果不存在，则返回 false 
	listSymbol := strings.Split(strFormatLogic, Space)
	for _, symbol := range listSymbol {
		if regex.MatchString(symbol) {
			// is id, check it
			if _, ok := mapExistIds[symbol]; ok {
				continue
			} else {
				return false
			}
		}
	}

	//把 strFormatLogic 切割得到的 ID 序列均存在于 map 中，则返回 true
	return true
}


func tryToCalculateResultByFormatLogicExpressionWithRandomProbe(strFormatLogic string) error {


	regex, err := regexp.Compile(PatternNumber)
	if err != nil {
		return err
	}

	listSymbol := strings.Split(strFormatLogic, Space)

	// random probe
	mapProbe := make(map[int]bool)
	for _, symbol := range listSymbol {
		if regex.MatchString(symbol) { 	
			//字符串 => 数字	
			id, iErr := strconv.Atoi(symbol) 	
			if iErr != nil {
				return iErr
			}
			//生成随机数
			randomInt := rand.Intn(10)
			randomBool := randomInt < 5
			//保存到 map
			mapProbe[id] = randomBool
		}
	}

	// calculate still use reverse_polish_notation
	r := &Rules{}
	_, err = r.calculateExpression(strFormatLogic, mapProbe)
	return err
}


// 查看操作符 op 是几元操作符
func numOfOperandInLogic(op string) int8 {
	mapOperand := map[string]int8{"or": 2, "and": 2, "not": 1}
	return mapOperand[op]
}

// 执行操作符所定义的操作
func computeOneInLogic(op string, v []bool) (bool, error) {
	switch op {
	case "or":
		return v[0] || v[1], nil
	case "and":
		return v[0] && v[1], nil
	case "not":
		return !v[0], nil
	default:
		return false, errors.New("unrecognized op")
	}
}

// 判断 needle 是否位于 haystack 集合中，如果设置了 isNeedleNum，则按照数值来比较，且允许小部分浮点误差。
func isIn(needle, haystack string, isNeedleNum bool) bool {
	// get number of needle
	var iNum float64
	var err error

	if isNeedleNum {
		if iNum, err = strconv.ParseFloat(needle, 64); err != nil {
			return false
		}
	}

	// compatible to "1, 2, 3" and "1,2,3"
	li := strings.Split(haystack, ",")
	for _, o := range li {
		trimO := strings.TrimLeft(o, " ")
		if isNeedleNum {
			oNum, err := strconv.ParseFloat(trimO, 64)
			if err != nil {
				continue
			}
			if math.Abs(iNum-oNum) < 1E-5 {
				// 考虑浮点精度问题
				return true
			}
		} else if needle == trimO {
			return true
		}
	}
	return false
}



// 判断 objStr 和 ruleStr 的两个集合是否有交集
func isIntersect(objStr string, ruleStr string) bool {
	// compatible to "1, 2, 3" and "1,2,3"
	vl := strings.Split(objStr, ",")
	li := strings.Split(ruleStr, ",")
	for _, o := range li {
		trimO := strings.Trim(o, " ")
		for _, v := range vl {
			trimV := strings.Trim(v, " ")
			if trimV == trimO {
				return true
			}
		}
	}
	return false
}


// 正则表达式中的特殊字符：
//
// ^ 匹配输入字符串的开始位置，除非在方括号表达式中使用，此时它表示不接受该字符集合。要匹配 ^ 字符本身，请使用 \^。
// $ 匹配输入字符串的结尾位置。如果设置了 RegExp 对象的 Multiline 属性，则 $ 也匹配 '\n' 或 '\r'。要匹配 $ 字符本身，请使用 \$。
// . 匹配除换行符 \n 之外的任何单字符。要匹配 .，请使用 \。
// \ 将下一个字符标记为或特殊字符、或原义字符、或后向引用、或八进制转义符。例如， 'n' 匹配字符 'n'。'\n' 匹配换行符。序列 '\\' 匹配 "\"，而 '\(' 则匹配 "("。
// | 指明两项之间的一个选择。要匹配 |，请使用 \|。
// { 标记限定符表达式的开始。要匹配 {，请使用 \{。
// [ 标记一个中括号表达式的开始。要匹配 [，请使用 \[。
// ( 和 ) 标记一个子表达式的开始和结束位置。子表达式可以获取供以后使用。要匹配这些字符，请使用 \( 和 \)。
// * 匹配前面的子表达式零次或多次。要匹配 * 字符，请使用 \*。
// + 匹配前面的子表达式一次或多次。要匹配 + 字符，请使用 \+。
// ? 匹配前面的子表达式零次或一次，或指明一个非贪婪限定符。要匹配 ? 字符，请使用 \?。

func isBetween(obj float64, scope string) bool {
	scope = strings.Trim(scope, " ")
	var equalLeft, equalRight bool
	// [] 双闭区间
	result := regexp.MustCompile("^\\[ *(-?\\d*.?\\d*) *, *(-?\\d*.?\\d*) *]$").FindStringSubmatch(scope)
	if len(result) > 2 {
		equalLeft = true
		equalRight = true
		return calculateBetween(obj, result, equalLeft, equalRight)
	}
	// [) 左闭右开区间
	result = regexp.MustCompile("^\\[ *(-?\\d*.?\\d*) *, *(-?\\d*.?\\d*) *\\)$").FindStringSubmatch(scope)
	if len(result) > 2 {
		equalLeft = true
		equalRight = false
		return calculateBetween(obj, result, equalLeft, equalRight)
	}
	// (] 左开右闭区间
	result = regexp.MustCompile("^\\( *(-?\\d*.?\\d*) *, *(-?\\d*.?\\d*) *]$").FindStringSubmatch(scope)
	if len(result) > 2 {
		equalLeft = false
		equalRight = true
		return calculateBetween(obj, result, equalLeft, equalRight)
	}
	// () 双开区间
	result = regexp.MustCompile("^\\( *(-?\\d*.?\\d*) *, *(-?\\d*.?\\d*) *\\)$").FindStringSubmatch(scope)
	if len(result) > 2 {
		equalLeft = false
		equalRight = false
		return calculateBetween(obj, result, equalLeft, equalRight)
	}
	return false
}


// 计算 obj 数值是否位于 result[1] 和 result[2] 定义的区间中
func calculateBetween(obj float64, result []string, equalLeft, equalRight bool) bool {
	var hasLeft, hasRight bool
	var left, right float64
	var err error
	if result[1] != "" {
		hasLeft = true
		left, err = strconv.ParseFloat(result[1], 64)
		if err != nil {
			return false
		}
	}
	if result[2] != "" {
		hasRight = true
		right, err = strconv.ParseFloat(result[2], 64)
		if err != nil {
			return false
		}
	}
	// calculate
	if !hasLeft && !hasRight {
		return false
	}
	flag := true
	if hasLeft {
		if equalLeft {
			flag = flag && obj >= left
		} else {
			flag = flag && obj > left
		}
	}
	if hasRight {
		if equalRight {
			flag = flag && obj <= right
		} else {
			flag = flag && obj < right
		}
	}
	return flag
}
