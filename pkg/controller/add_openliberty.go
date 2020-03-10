package controller

import (
	"github.com/OpenLiberty/open-liberty-operator/pkg/controller/openliberty"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, openliberty.Add)
}
