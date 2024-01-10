// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
//
// Code generated with OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package schema

// LdapWriteGroupRequest struct for LdapWriteGroupRequest
type LdapWriteGroupRequest struct {
	// Comma-separated list of policies associated to the group.
	Policies []string `json:"policies,omitempty"`
}
