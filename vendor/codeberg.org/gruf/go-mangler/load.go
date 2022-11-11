package mangler

import (
	"encoding"
	"net/url"
	"reflect"
	"time"
)

// loadMangler is the top-most Mangler load function. It guarantees that a Mangler
// function will be returned for given value interface{} and reflected type. Else panics.
func loadMangler(a any, t reflect.Type) Mangler {
	// Load mangler function
	mng, rmng := load(a, t)

	if rmng != nil {
		// Wrap reflect mangler to handle iface
		return func(buf []byte, a any) []byte {
			return rmng(buf, reflect.ValueOf(a))
		}
	}

	if mng == nil {
		// No mangler function could be determined
		panic("cannot mangle type: " + t.String())
	}

	return mng
}

// load will load a Mangler or reflect Mangler for given type and iface 'a'.
// Note: allocates new interface value if nil provided, i.e. if coming via reflection.
func load(a any, t reflect.Type) (Mangler, rMangler) {
	if t == nil {
		// There is no reflect type to search by
		panic("cannot mangle nil interface{} type")
	}

	if a == nil {
		// Alloc new iface instance
		v := reflect.New(t).Elem()
		a = v.Interface()
	}

	// Check in fast iface type switch
	if mng := loadIface(a); mng != nil {
		return mng, nil
	}

	// Search by reflection
	return loadReflect(t)
}

// loadIface is used as a first-resort interface{} type switcher loader
// for types implementing Mangled and providing performant alternative
// Mangler functions for standard library types to avoid reflection.
func loadIface(a any) Mangler {
	switch a.(type) {
	case Mangled:
		return mangle_mangled

	case time.Time:
		return mangle_time

	case *time.Time:
		return mangle_time_ptr

	case *url.URL:
		return mangle_stringer

	case encoding.BinaryMarshaler:
		return mangle_binary

	// NOTE:
	// we don't just handle ALL fmt.Stringer types as often
	// the output is large and unwieldy and this interface
	// switch is for types it would be faster to avoid reflection.
	// If they want better performance they can implement Mangled{}.

	default:
		return nil
	}
}

// loadReflect will load a Mangler (or rMangler) function for the given reflected type info.
// NOTE: this is used as the top level load function for nested reflective searches.
func loadReflect(t reflect.Type) (Mangler, rMangler) {
	switch t.Kind() {
	case reflect.Pointer:
		return loadReflectPtr(t.Elem())

	case reflect.String:
		return mangle_string, nil

	case reflect.Array:
		return nil, loadReflectArray(t.Elem())

	case reflect.Slice:
		// Element type
		et := t.Elem()

		// Preferably look for known slice mangler func
		if mng := loadReflectKnownSlice(et); mng != nil {
			return mng, nil
		}

		// Else handle as array elements
		return nil, loadReflectArray(et)

	case reflect.Map:
		return nil, loadReflectMap(t.Key(), t.Elem())

	case reflect.Bool:
		return mangle_bool, nil

	case reflect.Int,
		reflect.Uint,
		reflect.Uintptr:
		return mangle_platform_int, nil

	case reflect.Int8,
		reflect.Uint8:
		return mangle_8bit, nil

	case reflect.Int16,
		reflect.Uint16:
		return mangle_16bit, nil

	case reflect.Int32,
		reflect.Uint32:
		return mangle_32bit, nil

	case reflect.Int64,
		reflect.Uint64:
		return mangle_64bit, nil

	case reflect.Float32:
		return mangle_32bit, nil

	case reflect.Float64:
		return mangle_64bit, nil

	case reflect.Complex64:
		return mangle_64bit, nil

	case reflect.Complex128:
		return mangle_128bit, nil

	default:
		return nil, nil
	}
}

// loadReflectPtr loads a Mangler (or rMangler) function for a ptr's element type.
// This also handles further dereferencing of any further ptr indrections (e.g. ***int).
func loadReflectPtr(et reflect.Type) (Mangler, rMangler) {
	count := 1

	// Iteratively dereference ptrs
	for et.Kind() == reflect.Pointer {
		et = et.Elem()
		count++
	}

	if et.Kind() == reflect.Array {
		// Special case of addressable (sliceable) array
		if mng := loadReflectKnownSlice(et); mng != nil {
			if count == 1 {
				return mng, nil
			}
			return nil, deref_ptr_mangler(mng, count-1)
		}

		// Look for an array mangler function, this will
		// access elements by index using reflect.Value and
		// pass each one to a separate mangler function.
		if rmng := loadReflectArray(et); rmng != nil {
			return nil, deref_ptr_rmangler(rmng, count)
		}

		return nil, nil
	}

	// Try remove a layer of derefs by loading a mangler
	// for a known ptr kind. The less reflection the better!
	if mng := loadReflectKnownPtr(et); mng != nil {
		if count == 1 {
			return mng, nil
		}
		return nil, deref_ptr_mangler(mng, count-1)
	}

	// Search for ptr elemn type mangler
	if mng, rmng := load(nil, et); mng != nil {
		return nil, deref_ptr_mangler(mng, count)
	} else if rmng != nil {
		return nil, deref_ptr_rmangler(rmng, count)
	}

	return nil, nil
}

// loadReflectKnownPtr loads a Mangler function for a known ptr-of-element type (in this case, primtive ptrs).
func loadReflectKnownPtr(et reflect.Type) Mangler {
	switch et.Kind() {
	case reflect.String:
		return mangle_string_ptr

	case reflect.Bool:
		return mangle_bool_ptr

	case reflect.Int,
		reflect.Uint,
		reflect.Uintptr:
		return mangle_platform_int_ptr

	case reflect.Int8,
		reflect.Uint8:
		return mangle_8bit_ptr

	case reflect.Int16,
		reflect.Uint16:
		return mangle_16bit_ptr

	case reflect.Int32,
		reflect.Uint32:
		return mangle_32bit_ptr

	case reflect.Int64,
		reflect.Uint64:
		return mangle_64bit_ptr

	case reflect.Float32:
		return mangle_32bit_ptr

	case reflect.Float64:
		return mangle_64bit_ptr

	case reflect.Complex64:
		return mangle_64bit_ptr

	case reflect.Complex128:
		return mangle_128bit_ptr

	default:
		return nil
	}
}

// loadReflectKnownSlice loads a Mangler function for a known slice-of-element type (in this case, primtives).
func loadReflectKnownSlice(et reflect.Type) Mangler {
	switch et.Kind() {
	case reflect.String:
		return mangle_string_slice

	case reflect.Bool:
		return mangle_bool_slice

	case reflect.Int,
		reflect.Uint,
		reflect.Uintptr:
		return mangle_platform_int_slice

	case reflect.Int8,
		reflect.Uint8:
		return mangle_8bit_slice

	case reflect.Int16,
		reflect.Uint16:
		return mangle_16bit_slice

	case reflect.Int32,
		reflect.Uint32:
		return mangle_32bit_slice

	case reflect.Int64,
		reflect.Uint64:
		return mangle_64bit_slice

	case reflect.Float32:
		return mangle_32bit_slice

	case reflect.Float64:
		return mangle_64bit_slice

	case reflect.Complex64:
		return mangle_64bit_slice

	case reflect.Complex128:
		return mangle_128bit_slice

	default:
		return nil
	}
}

// loadReflectArray loads an rMangler function for an array (or slice) or given element type.
func loadReflectArray(et reflect.Type) rMangler {
	// Search via reflected array element type
	if mng, rmng := load(nil, et); mng != nil {
		return iter_array_mangler(mng)
	} else if rmng != nil {
		return iter_array_rmangler(rmng)
	}
	return nil
}

// loadReflectMap ...
func loadReflectMap(kt, vt reflect.Type) rMangler {
	var kmng, vmng rMangler

	// Search for mangler for key type
	mng, rmng := load(nil, kt)

	switch {
	// Wrap key mangler to reflect
	case mng != nil:
		mng := mng // take our own ptr
		kmng = func(buf []byte, v reflect.Value) []byte {
			return mng(buf, v.Interface())
		}

	// Use reflect key mangler as-is
	case rmng != nil:
		kmng = rmng

	// No mangler found
	default:
		return nil
	}

	// Search for mangler for value type
	mng, rmng = load(nil, vt)

	switch {
	// Wrap key mangler to reflect
	case mng != nil:
		mng := mng // take our own ptr
		vmng = func(buf []byte, v reflect.Value) []byte {
			return mng(buf, v.Interface())
		}

	// Use reflect key mangler as-is
	case rmng != nil:
		vmng = rmng

	// No mangler found
	default:
		return nil
	}

	// Wrap key/value manglers in map iter
	return iter_map_rmangler(kmng, vmng)
}
