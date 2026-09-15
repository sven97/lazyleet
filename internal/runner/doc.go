// Package runner is the local judge: per-language drivers that wrap the user's
// solution, parse test-case input, invoke it in a sandboxed subprocess, and
// compare output. Supported languages: python3, javascript, golang, java, cpp.
package runner
