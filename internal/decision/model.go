package decision

// Model is the single source of truth for an active engine's API policy.
type Model struct {
	ID                                         string
	Aliases                                    []string
	Chinese                                    bool
	Description, AliasDescription, ReleaseDate string
}

func JuliaModel() Model {
	return Model{ID: "julia-1", Aliases: []string{"jev-latest", "jev-1.13.0", "typesafe/jev-1.13"}, Description: "Local Julia-1 ONNX CPU decision model", AliasDescription: "Local alias for Julia-1", ReleaseDate: "2026-09-25"}
}
func ChineseModel() Model {
	return Model{ID: "chinese-decision-2b", Chinese: true, Description: "Local Chinese Decision 2B Q8_0 Metal decision model"}
}
func (m Model) Listings() []map[string]string {
	out := []map[string]string{}
	for i, name := range append([]string{m.ID}, m.Aliases...) {
		description := m.Description
		if i > 0 {
			description = m.AliasDescription
		}
		item := map[string]string{"name": name, "description": description}
		if m.ReleaseDate != "" {
			item["release_date"] = m.ReleaseDate
		}
		out = append(out, item)
	}
	return out
}
func (m Model) Accepts(name string) bool {
	if name == m.ID {
		return true
	}
	for _, alias := range m.Aliases {
		if name == alias {
			return true
		}
	}
	return false
}
func (s *Service) ModelInfo() Model {
	if s.Model.ID == "" {
		return JuliaModel()
	}
	return s.Model
}
func ParseForModel(body []byte, maxQuestions int, model Model) (Request, error) {
	if model.Chinese {
		return parseChinese(body, maxQuestions)
	}
	return ParseRequest(body, maxQuestions)
}
