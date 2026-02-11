// Throwaway POC to verify glamour markdown rendering.
// Run: go run cmd/mdtest/main.go
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"golang.org/x/term"
)

func main() {
	md := `### Next Steps

Would you like me to:

- Push the 2 unpushed commits?
- Run the test suite to verify everything?
- Review the TODO items?
- Check for any missing documentation?
`
	w := 80
	if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 0 {
		w = tw
		fmt.Printf("[term width detected: %d]\n", w)
	} else {
		fmt.Printf("[term.GetSize failed: %v, using default %d]\n", err, w)
	}

	s := glamourstyles.DarkStyleConfig
	s.H2.Prefix = ""
	s.H3.Prefix = ""
	s.H4.Prefix = ""
	s.H5.Prefix = ""
	s.H6.Prefix = ""
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(s),
		glamour.WithWordWrap(w),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "glamour init error: %v\n", err)
		os.Exit(1)
	}

	rendered, err := r.Render(md)
	if err != nil {
		fmt.Fprintf(os.Stderr, "glamour render error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== RAW ===")
	fmt.Print(md)
	fmt.Println("=== GLAMOUR ===")
	fmt.Print(rendered)
	fmt.Println("=== END ===")
}
