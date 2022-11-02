package test_package_path_not_import_path

import (
	"fmt"
	v1 "gopkg.in/square/go-jose.v1"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

func Whatever() {
	blah := jwt.JSONWebToken{}

	fmt.Println(blah.Headers, jose.A192GCM, v1.A192GCM)
}
