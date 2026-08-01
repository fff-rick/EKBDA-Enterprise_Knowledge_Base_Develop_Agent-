package development

var commandCatalog = map[string]Command{
	"git-diff-check": {ID: "git-diff-check", Executable: "git", Arguments: []string{"diff", "--check"}, Purpose: "检查补丁空白与冲突标记"},
	"go-test-all":    {ID: "go-test-all", Executable: "go", Arguments: []string{"test", "./..."}, Purpose: "运行全部 Go 测试"},
	"go-vet-all":     {ID: "go-vet-all", Executable: "go", Arguments: []string{"vet", "./..."}, Purpose: "运行 Go 静态检查"},
	"go-build-all":   {ID: "go-build-all", Executable: "go", Arguments: []string{"build", "./..."}, Purpose: "构建全部 Go 包"},
}

func CommandCatalog() []Command {
	ids := []string{"git-diff-check", "go-test-all", "go-vet-all", "go-build-all"}
	result := make([]Command, 0, len(ids))
	for _, id := range ids {
		command := commandCatalog[id]
		command.Arguments = append([]string(nil), command.Arguments...)
		result = append(result, command)
	}
	return result
}
