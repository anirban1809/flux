package viewctx

import (
	"flux/src/agent"

	"github.com/anirban1809/tuix/tuix"
)

type ContextType struct {
	Runtime        *agent.Runtime
	SetFocusPrompt func(bool)
}

var MainContext = tuix.CreateContext[*ContextType](nil)
