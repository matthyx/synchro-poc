package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/SergJa/jsonhash"
	"github.com/davecgh/go-spew/spew"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/stretchr/testify/assert"
)

func CanonicalHash(in []byte) ([32]byte, error) {
	return jsonhash.CalculateJsonHash(in, []string{
		".status.conditions", // avoid Pod.status.conditions.lastProbeTime: null
	})
}

func CompareJson(a, b []byte) bool {
	var aData interface{}
	var bData interface{}
	err := json.Unmarshal(a, &aData)
	if err != nil {
		logger.L().Error("cannot unmarshal a", helpers.Error(err))
		return false
	}
	err = json.Unmarshal(b, &bData)
	if err != nil {
		logger.L().Error("cannot unmarshal b", helpers.Error(err))
		return false
	}
	equal := assert.ObjectsAreEqual(aData, bData)
	if !equal {
		fmt.Println(diff(aData, bData))
	}
	return equal
}

func diff(expected interface{}, actual interface{}) string {
	if expected == nil || actual == nil {
		return ""
	}

	et, ek := typeAndKind(expected)
	at, _ := typeAndKind(actual)

	if et != at {
		return ""
	}

	if ek != reflect.Struct && ek != reflect.Map && ek != reflect.Slice && ek != reflect.Array && ek != reflect.String {
		return ""
	}

	var e, a string

	switch et {
	case reflect.TypeOf(""):
		e = reflect.ValueOf(expected).String()
		a = reflect.ValueOf(actual).String()
	case reflect.TypeOf(time.Time{}):
		e = spewConfigStringerEnabled.Sdump(expected)
		a = spewConfigStringerEnabled.Sdump(actual)
	default:
		e = spewConfig.Sdump(expected)
		a = spewConfig.Sdump(actual)
	}

	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(e),
		B:        difflib.SplitLines(a),
		FromFile: "Expected",
		FromDate: "",
		ToFile:   "Actual",
		ToDate:   "",
		Context:  1,
	})

	return "\n\nDiff:\n" + diff
}

var spewConfig = spew.ConfigState{
	Indent:                  " ",
	DisablePointerAddresses: true,
	DisableCapacities:       true,
	SortKeys:                true,
	DisableMethods:          true,
	MaxDepth:                10,
}

var spewConfigStringerEnabled = spew.ConfigState{
	Indent:                  " ",
	DisablePointerAddresses: true,
	DisableCapacities:       true,
	SortKeys:                true,
	MaxDepth:                10,
}

func typeAndKind(v interface{}) (reflect.Type, reflect.Kind) {
	t := reflect.TypeOf(v)
	k := t.Kind()

	if k == reflect.Ptr {
		t = t.Elem()
		k = t.Kind()
	}
	return t, k
}
