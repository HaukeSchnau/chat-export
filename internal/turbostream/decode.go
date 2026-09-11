// Package turbostream decodes the wire format of the turbo-stream library
// (github.com/jacob-ebey/turbo-stream v2), which React Router uses to embed
// loader data in server-rendered HTML.
//
// The format is a flat JSON array. Every value in the array is either a
// primitive, an object whose keys are "_<index>" and whose values are indexes,
// a plain array of indexes, or a tagged array whose first element is a
// type letter (for example ["D", 3] for a Date). Negative indexes are
// sentinels for undefined, null, NaN and friends.
//
// Only the first line of a stream is decoded. Later lines resolve deferred
// promises, which the ChatGPT share page uses solely for telemetry.
package turbostream

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Sentinel indexes, mirroring utils.ts in turbo-stream.
const (
	hole             = -1
	nan              = -2
	negativeInfinity = -3
	negativeZero     = -4
	null             = -5
	positiveInfinity = -6
	undefined        = -7
)

// Decode hydrates the root value of a turbo-stream payload. Objects become
// map[string]any, arrays become []any, numbers become float64. Dates and URLs
// are returned as their string form. Promises decode to nil.
func Decode(payload string) (any, error) {
	firstLine, _, _ := strings.Cut(payload, "\n")

	var values []any
	if err := json.Unmarshal([]byte(firstLine), &values); err != nil {
		return nil, fmt.Errorf("turbostream: root line is not a JSON array: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("turbostream: empty payload")
	}

	d := &decoder{values: values, hydrated: map[int]any{}}
	return d.hydrate(0)
}

type decoder struct {
	values   []any
	hydrated map[int]any
}

func (d *decoder) hydrate(index int) (any, error) {
	switch index {
	case undefined, null:
		return nil, nil
	case nan:
		return math.NaN(), nil
	case positiveInfinity:
		return math.Inf(1), nil
	case negativeInfinity:
		return math.Inf(-1), nil
	case negativeZero:
		return math.Copysign(0, -1), nil
	}
	if index < 0 || index >= len(d.values) {
		return nil, fmt.Errorf("turbostream: index %d out of range", index)
	}
	if v, ok := d.hydrated[index]; ok {
		return v, nil
	}

	switch value := d.values[index].(type) {
	case map[string]any:
		return d.hydrateObject(index, value)
	case []any:
		if len(value) > 0 {
			if tag, ok := value[0].(string); ok {
				return d.hydrateTagged(index, tag, value[1:])
			}
		}
		return d.hydrateArray(index, value)
	default:
		d.hydrated[index] = value
		return value, nil
	}
}

func (d *decoder) hydrateObject(index int, raw map[string]any) (any, error) {
	out := make(map[string]any, len(raw))
	d.hydrated[index] = out // registered before recursing so cycles terminate
	for rawKey, rawValue := range raw {
		keyIndex, err := strconv.Atoi(strings.TrimPrefix(rawKey, "_"))
		if err != nil {
			return nil, fmt.Errorf("turbostream: malformed object key %q", rawKey)
		}
		key, err := d.hydrate(keyIndex)
		if err != nil {
			return nil, err
		}
		keyString, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("turbostream: object key at %d is not a string", keyIndex)
		}
		valueIndex, err := asIndex(rawValue)
		if err != nil {
			return nil, err
		}
		val, err := d.hydrate(valueIndex)
		if err != nil {
			return nil, err
		}
		out[keyString] = val
	}
	return out, nil
}

func (d *decoder) hydrateArray(index int, raw []any) (any, error) {
	out := make([]any, len(raw))
	d.hydrated[index] = out
	for i, rawValue := range raw {
		elemIndex, err := asIndex(rawValue)
		if err != nil {
			return nil, err
		}
		if elemIndex == hole {
			continue
		}
		if out[i], err = d.hydrate(elemIndex); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (d *decoder) hydrateTagged(index int, tag string, args []any) (any, error) {
	switch tag {
	case "P": // promise: resolved on a later stream line, not needed here
		d.hydrated[index] = nil
		return nil, nil
	case "Z": // previously resolved promise value
		ref, err := asIndex(args[0])
		if err != nil {
			return nil, err
		}
		return d.hydrate(ref)
	case "D", "U", "B", "R", "Y", "E": // date, url, bigint, regexp, symbol, error
		d.hydrated[index] = args[0]
		return args[0], nil
	case "N": // null-prototype object
		raw, ok := args[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("turbostream: malformed null object at %d", index)
		}
		return d.hydrateObject(index, raw)
	case "S": // set
		return d.hydrateArray(index, args)
	case "M": // map: alternating key and value indexes
		out := make(map[string]any, len(args)/2)
		d.hydrated[index] = out
		for i := 0; i+1 < len(args); i += 2 {
			keyIndex, err := asIndex(args[i])
			if err != nil {
				return nil, err
			}
			key, err := d.hydrate(keyIndex)
			if err != nil {
				return nil, err
			}
			valueIndex, err := asIndex(args[i+1])
			if err != nil {
				return nil, err
			}
			val, err := d.hydrate(valueIndex)
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(key)] = val
		}
		return out, nil
	default:
		return nil, fmt.Errorf("turbostream: unknown tag %q at %d", tag, index)
	}
}

func asIndex(v any) (int, error) {
	f, ok := v.(float64)
	if !ok || f != math.Trunc(f) {
		return 0, fmt.Errorf("turbostream: expected index, got %v", v)
	}
	return int(f), nil
}
