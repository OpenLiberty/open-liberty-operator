package socket

import (
	"fmt"
	"reflect"
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func verifyTests(tests []Test) error {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			return fmt.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
	return nil
}
