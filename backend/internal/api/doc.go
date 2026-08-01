// Package api is the HTTP transport layer: routing, request decoding,
// response encoding, and middleware.
//
// Handlers translate HTTP into calls on the domain packages and translate
// results back. Business logic does not belong here.
package api
