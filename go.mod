module gopkg.in/dnaeon/go-vcr.v3

go 1.19

require gopkg.in/yaml.v3 v3.0.1

retract (
	v3.2.1 // Default Matcher is stricter
	v3.2.2 // Contains retractions only
)
