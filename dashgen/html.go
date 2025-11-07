package dashgen

import (
	"bytes"
	"fmt"
	"github.com/maragudk/gomponents"
	"github.com/maragudk/gomponents/html"
)

// StandardHeader creates the consistent VeChain dashboard header panel content
// with the standard gradient styling and typography (36px font size)
func StandardHeader(title string) string {
	var buf bytes.Buffer
	err := html.Div(
		html.Style("display: flex; align-items: center; justify-content: center; width: 100%; height: 48px; padding: 0 16px; border-radius: 10px; background: linear-gradient(90deg, #02254A 0%, #03422C 100%); color: #e9fff9; font: 600 36px/1.1 Inter, system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif; letter-spacing: .08em; text-transform: uppercase; text-align: center; box-shadow: inset 0 0 0 1px rgba(255, 255, 255, .06), 0 6px 20px rgba(0, 0, 0, .35);"),
		gomponents.Text(title),
	).Render(&buf)
	if err != nil {
		fmt.Println("Error: ", err)
	}
	return buf.String()
}
