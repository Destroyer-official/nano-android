package main

import (
	"fmt"
	"os"
	"path/filepath"

	"nanoandroid/engine/ast_input"
)

func main() {
	xmlFiles := []string{"Studio.xml", "Duo.xml", "Matrix.xml", "Settings.xml", "Tools.xml"}
	service := ast_input.NewASTService()

	fmt.Println("================================================================================")
	fmt.Println("🔍 TESTING REAL APK XML EXTRACTION WITH AST SERVICE")
	fmt.Println("================================================================================")

	for _, file := range xmlFiles {
		p := filepath.Join(".", file)
		if _, err := os.Stat(p); err != nil {
			p = filepath.Join("..", file)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Printf("❌ Failed to read %s: %v\n", file, err)
			continue
		}

		st, err := service.ExtractAST(data)
		if err != nil {
			fmt.Printf("❌ Failed to parse %s: %v\n", file, err)
			continue
		}

		fmt.Printf("\n📄 Screen: %s (%d bytes XML)\n", file, len(data))
		fmt.Printf("   • Raw nodes: %d | Pruned: %d | Semantic Nodes: %d | Tokens: ~%d\n",
			st.TotalRawNodes, st.PrunedCount, len(st.Nodes), st.TokenEstimate)
		
		fmt.Printf("   • Sample extracted interactive nodes:\n")
		count := 0
		for _, n := range st.Nodes {
			if n.Text != "" || n.Clickable {
				fmt.Printf("     - [%s] %s %q bounds:%v (clickable: %v)\n",
					n.ID, n.Class, n.Text, n.Bounds, n.Clickable)
				count++
				if count >= 6 {
					break
				}
			}
		}
	}
}
