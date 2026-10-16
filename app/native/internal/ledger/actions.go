package ledger

import "encoding/json"

func textResult(value any) json.RawMessage {
	text, _ := json.Marshal(value)
	result, _ := json.Marshal(map[string]any{"content": []any{map[string]string{"type": "text", "text": string(text)}}})
	return result
}
func actionCapabilities(w Work) json.RawMessage {
	tools := []PluginTool{}
	for _, p := range policyBuiltinPlugins(w.ToolPolicyVersion) {
		for _, tool := range p.Tools {
			if !pinnedTool(w, tool.Name) || tool.Name == "artifact.write_text" && !stageWriteAllowed(w) || tool.Name == "project.read_file" && (w.Resources == nil || len(w.Resources.Files) == 0) || tool.Name == "report.read_artifact" && !hasPublishedArtifacts(w) {
				continue
			}
			tools = append(tools, tool)
		}
	}
	return textResult(map[string]any{"tools": tools, "scope": "local task only; external actions disabled"})
}
