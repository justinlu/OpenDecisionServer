package decision

// Model is the single source of truth for an active engine's API policy.
type Model struct {
	ID      string
	Aliases []string
	Chinese bool
}

func JuliaModel() Model {
	return Model{ID: "julia-1", Aliases: []string{"jev-latest", "jev-1.13.0", "typesafe/jev-1.13"}}
}
func ChineseModel() Model { return Model{ID: "chinese-decision-2b", Chinese: true} }
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
