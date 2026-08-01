// Package execution is the workflow engine.
//
// It walks a workflow graph, runs each node, tracks state, passes variables
// between nodes, handles retries and branching, and records logs.
//
// The engine knows nothing about HTTP or the UI. It is driven by the api
// package and reports progress through the websocket package.
package execution
