package action

// Registry is keyed by the kebab-case action ID. Each verb file populates
// itself via an init().
var Registry = map[string]Action{}
