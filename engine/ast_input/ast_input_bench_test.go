package ast_input_test

import (
	"testing"

	"nanoandroid/engine/ast_input"
)

func buildBenchmarkViewTree() *ast_input.View {
	root := ast_input.NewView("root", "android.widget.FrameLayout", [4]int{0, 0, 1080, 2400})
	coordinator := ast_input.NewView("coord", "androidx.coordinatorlayout.widget.CoordinatorLayout", [4]int{0, 0, 1080, 2400})
	root.AddChild(coordinator)

	content := ast_input.NewView("content", "android.widget.LinearLayout", [4]int{0, 100, 1080, 2400})
	coordinator.AddChild(content)

	for i := 0; i < 10; i++ {
		row := ast_input.NewView(ast_input.NextNodeID(), "android.widget.LinearLayout", [4]int{0, 100 + i*100, 1080, 200 + i*100})
		lbl := ast_input.NewView(ast_input.NextNodeID(), "android.widget.TextView", [4]int{50, 110 + i*100, 400, 190 + i*100})
		lbl.Text = "Option Item"
		btn := ast_input.NewView(ast_input.NextNodeID(), "android.widget.Button", [4]int{500, 110 + i*100, 900, 190 + i*100})
		btn.Text = "Action"
		btn.Clickable = true
		row.AddChild(lbl)
		row.AddChild(btn)
		content.AddChild(row)
	}
	return root
}

func BenchmarkExtractAST(b *testing.B) {
	root := buildBenchmarkViewTree()
	service := ast_input.NewASTService()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := service.ExtractAST(root)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDispatchTap(b *testing.B) {
	root := buildBenchmarkViewTree()
	service := ast_input.NewASTService()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = service.DispatchTap(root, "node_3")
	}
}

func BenchmarkDiffAST(b *testing.B) {
	root := buildBenchmarkViewTree()
	service := ast_input.NewASTService()
	t1, _ := service.ExtractAST(root)

	// Modify one node
	if len(root.Children[0].Children[0].Children) > 0 {
		btn := root.Children[0].Children[0].Children[0].Children[1]
		btn.Text = "Updated Action"
	}
	t2, _ := service.ExtractAST(root)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ast_input.DiffAST(t1, t2)
	}
}

func BenchmarkParseAndExtractXML(b *testing.B) {
	xmlData := []byte(`<?xml version="1.0" encoding="utf-8"?><hierarchy rotation="0">
<node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
  <node class="android.widget.LinearLayout" bounds="[0,0][1080,2400]">
    <node resource-id="com.app:id/title" class="android.widget.TextView" text="Hello Bench" bounds="[50,100][500,200]" />
    <node resource-id="com.app:id/btn" class="android.widget.Button" text="Click" clickable="true" bounds="[50,220][500,340]" />
  </node>
</node></hierarchy>`)

	service := ast_input.NewASTService()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := service.ExtractAST(xmlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}
