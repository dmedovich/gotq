// Package query provides a safe query engine for GORM list endpoints. It parses
// bounded HTTP queries, validates scalar and relationship paths against explicit
// endpoint policy, and either derives GORM scopes or executes stable offset or
// forward-cursor pages without multiplying roots for to-many predicates.
package query
