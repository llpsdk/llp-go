package llp

import "context"

type Annotater interface {
	AnnotateToolCall(ctx context.Context, toolCall ToolCall) error
}
