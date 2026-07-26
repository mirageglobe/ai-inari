// keys.go owns detecting config keys that no Config field accepts. it does NOT
// own loading or defaulting (config.go) or reporting the result (inari doctor).

package config

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
)

// UnknownKeys returns the json keys in the config file at path that no Config
// field accepts, as sorted dotted paths ("models.worker"). the decoder silently
// drops such keys, so a config carrying a renamed field looks configured while
// having no effect: `models.worker` survived the worker/runner consolidation in
// live configs and quietly left `models.runner` empty. a missing or malformed
// file yields no keys and no error, since Load already reports those.
func UnknownKeys(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, nil
	}
	var out []string
	walkKeys("", m, reflect.TypeOf(Config{}), &out)
	sort.Strings(out)
	return out, nil
}

// walkKeys compares one decoded json object against the struct that receives it,
// recording unrecognised keys and descending into nested objects.
func walkKeys(prefix string, m map[string]any, t reflect.Type, out *[]string) {
	for key, val := range m {
		field, ok := fieldForJSONName(t, key)
		if !ok {
			*out = append(*out, prefix+key)
			continue
		}
		descend(prefix+key+".", val, deref(field.Type), out)
	}
}

// descend recurses into whatever shape the field holds: a nested object, a map of
// objects (profile names are user-chosen, so map keys are never unknown fields),
// or a list of objects.
func descend(prefix string, val any, ft reflect.Type, out *[]string) {
	switch ft.Kind() {
	case reflect.Struct:
		if child, ok := val.(map[string]any); ok {
			walkKeys(prefix, child, ft, out)
		}
	case reflect.Map:
		child, ok := val.(map[string]any)
		if !ok || deref(ft.Elem()).Kind() != reflect.Struct {
			return
		}
		for name, entry := range child {
			descend(prefix+name+".", entry, deref(ft.Elem()), out)
		}
	case reflect.Slice:
		items, ok := val.([]any)
		if !ok || deref(ft.Elem()).Kind() != reflect.Struct {
			return
		}
		for _, item := range items {
			descend(prefix, item, deref(ft.Elem()), out)
		}
	}
}

// fieldForJSONName finds the struct field whose json tag names key. the tag is
// authoritative: field names and json keys differ throughout Config.
func fieldForJSONName(t reflect.Type, key string) (reflect.StructField, bool) {
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" {
			name = f.Name
		}
		if name == key {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

func deref(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}
