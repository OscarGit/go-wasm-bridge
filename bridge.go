package wasmbridge

import (
	"fmt"
	"os"
	"syscall/js"
)

var (
	global   js.Value
	goBridge js.Value
)

func init() {
	if len(os.Args) != 2 {
		panic("Expected two arguments from os.Args")
	}
	bridgeName := os.Args[1]
	global := js.Global()
	goBridge = global.Get(bridgeName)
}

// Will convert a js.Value to a interface acording to the mapping below
//  Attributes of object and elements of arrays will also be converted with this function
//  | js.Value					| interface{}				|
//  |---------------------------|---------------------------|
//  | undefined					| nil						|
//  | null						| nil						|
//  | boolean					| bool						|
//  | string					| string					|
//  | number					| float64					|
//  | bigint					| int						|
//  | object					| map[string]interface{}	|
//  | Array (obj)				| []interface{}				|
//  | Uint8Array (obj)			| []byte					|
//  | Uint8ClampedArray (obj)	| []byte					|
//
func jsToInterface(value js.Value) interface{} {
	primType := value.Type().String()
	switch primType {
	case "undefined":
		return nil
	case "null":
		return nil
	case "boolean":
		return value.Bool()
	case "string":
		return value.String()
	case "number":
		return value.Float()
	case "bigint":
		return value.Int()
	case "object":
		objType := value.Get("constructor").Get("name").String()
		switch objType {
		case "Object":
			return jsObjToMap(value)
		case "Array":
			return jsArrayToArray(value)
		case "Uint8Array":
			fallthrough
		case "Uint8ClampedArray":
			data := make([]byte, value.Length())
			js.CopyBytesToGo(data, value)
			return data
		default:
			panic(fmt.Sprintf("Object type not supported in wasmbridge: %s", objType))
		}
	default:
		panic(fmt.Sprintf("Primitive type not supported in wasmbridge: %s", primType))
	}
}
func jsArrayToArray(array js.Value) []interface{} {
	res := make([]interface{}, array.Length())
	for i := range res {
		res[i] = jsToInterface(array.Index(i))
	}
	return res
}
func jsObjToMap(object js.Value) map[string]interface{} {
	res := map[string]interface{}{}
	keys := js.Global().Get("Object").Call("keys", object)
	for i := 0; i < keys.Length(); i++ {
		key := keys.Index(i).String()
		attr := object.Get(key)
		res[key] = jsToInterface(attr)
	}
	return res
}

// ExportFunc - Export function to JS
func ExportFunc(name string, goFn func([]interface{}) (interface{}, error)) {
	goBridge.Set(name, js.FuncOf(func(this js.Value, jsArgs []js.Value) interface{} {
		goArgs := make([]interface{}, len(jsArgs))

		for i := range jsArgs {
			varType := jsArgs[i].Type().String()
			fmt.Printf("Arg %d: %s\n", i, varType)
			goArgs[i] = jsToInterface(jsArgs[i])
		}

		ret, err := goFn(goArgs)

		if err != nil {
			this.Set("error", err.Error())
			return 1
		}

		data, isByteSlice := ret.([]byte)
		if isByteSlice {
			jsArray := js.Global().Get("Uint8ClampedArray").New(js.ValueOf(len(data)))
			js.CopyBytesToJS(jsArray, data)
			this.Set("result", jsArray)
		} else {
			this.Set("result", ret)
		}

		return 0
	}))
}
