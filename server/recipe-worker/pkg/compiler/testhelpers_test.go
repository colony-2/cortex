package compiler

func withRequiredGitInputs(inputs map[string]interface{}) map[string]interface{} {
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	if _, ok := inputs["basegitrepo"]; !ok {
		inputs["basegitrepo"] = "/tmp/vibethis/test-repo"
	}
	if _, ok := inputs["basegithash"]; !ok {
		inputs["basegithash"] = "0000000000000000000000000000000000000000"
	}
	if _, ok := inputs["ticketid"]; !ok {
		inputs["ticketid"] = "TEST-TICKET"
	}
	if _, ok := inputs["cellname"]; !ok {
		inputs["cellname"] = "test-cell"
	}

	return inputs
}
