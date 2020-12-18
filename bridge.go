// +build js,wasm

package jsbridge

import (
	"fmt"
	"os"
	"syscall/js"
)

const (
	jsBridgeName = "__jsbridge"
)

var (
	global       js.Value
	moduleBridge js.Value
)

func init() {
	if len(os.Args) < 2 {
		panic("Expected two arguments from os.Args")
	}
	bridgeName := os.Args[1]
	global := js.Global()
	moduleBridge = global.Get(jsBridgeName).Get(bridgeName)
}

// Will convert a js.Value to a interface acording to the mapping below
//  Attributes of object and elements of arrays will also be converted with this function
//  | js.Value                  | interface{}            |
//  |---------------------------|------------------------|
//  | undefined                 | nil                    |
//  | null                      | nil                    |
//  | boolean                   | bool                   |
//  | string                    | string                 |
//  | number                    | float64                |
//  | bigint                    | int                    |
//  | object                    | map[string]interface{} |
//  | Array (obj)               | []interface{}          |
//  | Uint8Array (obj)          | []byte                 |
//  | Uint8ClampedArray (obj)   | []byte                 |
//
func jsToInterface(value js.Value) (interface{}, error) {
	primType := value.Type().String()
	switch primType {
	case "number":
		return value.Float(), nil
	case "bigint":
		return value.Int(), nil
	case "undefined":
		return nil, nil
	case "null":
		return nil, nil
	case "boolean":
		return value.Bool(), nil
	case "string":
		return value.String(), nil
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
			return data, nil
		default:
			return nil, fmt.Errorf("Object type not supported in wasmbridge: %s", objType)
		}
	default:
		return nil, fmt.Errorf("Primitive type not supported in wasmbridge: %s", primType)
	}
}
func jsArrayToArray(array js.Value) ([]interface{}, error) {
	res := make([]interface{}, array.Length())
	for i := range res {
		value, err := jsToInterface(array.Index(i))
		if err != nil {
			return nil, err
		}
		res[i] = value
	}
	return res, nil
}
func jsObjToMap(object js.Value) (map[string]interface{}, error) {
	res := map[string]interface{}{}
	keys := js.Global().Get("Object").Call("keys", object)
	for i := 0; i < keys.Length(); i++ {
		key := keys.Index(i).String()
		value, err := jsToInterface(object.Get(key))
		if err != nil {
			return nil, err
		}
		res[key] = value
	}
	return res, nil
}

func interfaceToJs(value interface{}, useClamped bool) js.Value {
	// Check if result is a byte slice
	data, isByteSlice := value.([]byte)
	if isByteSlice {
		return byteSliceToJs(data, useClamped)
	}
	return js.ValueOf(value)
}

// Will convert a byte slice to a JS Uint8 array
func byteSliceToJs(data []byte, useClamped bool) js.Value {
	var arrayType string
	if useClamped {
		arrayType = "Uint8ClampedArray"
	} else {
		arrayType = "Uint8Array"
	}

	jsArray := js.Global().Get(arrayType).New(js.ValueOf(len(data)))
	js.CopyBytesToJS(jsArray, data)
	return jsArray
}

// ExportFunc - Export function to JS
func ExportFunc(name string, goFn func([]interface{}) (interface{}, error), useClamped bool) {
	moduleBridge.Set(name, js.FuncOf(func(this js.Value, jsArgs []js.Value) interface{} {
		goArgs := make([]interface{}, len(jsArgs))

		// Convert arguments to GO interface
		for i := range jsArgs {
			value, err := jsToInterface(jsArgs[i])
			if err != nil {
				this.Set("error", err.Error())
				return nil
			}
			goArgs[i] = value
		}

		// Make call to function
		ret, err := goFn(goArgs)
		if err != nil {
			this.Set("error", err.Error())
			return nil
		}

		// Convert and set result
		this.Set("result", interfaceToJs(ret, useClamped))
		return nil
	}))
}
