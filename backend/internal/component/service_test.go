package component

import "testing"

func TestValidateSystemComponentTransitionProtectsArticleInfoboxDeprecation(t *testing.T) {
	if err := validateSystemComponentTransition(&Component{ComponentKey: BuiltinArticleInfoboxKey}, StatusDeprecated); err == nil {
		t.Fatal("article-infobox deprecation was allowed")
	}
}

func TestValidateSystemComponentTransitionAllowsOtherTransitions(t *testing.T) {
	tests := []struct {
		name      string
		component *Component
		target    string
	}{
		{name: "article infobox publish", component: &Component{ComponentKey: BuiltinArticleInfoboxKey}, target: StatusPublished},
		{name: "custom deprecate", component: &Component{ComponentKey: "custom-card"}, target: StatusDeprecated},
		{name: "nil definition", component: nil, target: StatusDeprecated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSystemComponentTransition(test.component, test.target); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
