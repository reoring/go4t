package rt

import "regexp"

// RegexMatch searches for a match; callers can anchor the pattern for a full match.
func RegexMatch(pattern, s any) any {
	re, err := regexp.Compile(pattern.(string))
	return err == nil && re.MatchString(s.(string))
}

func RegexFind(pattern, s any) any {
	re, err := regexp.Compile(pattern.(string))
	if err != nil {
		return NoneOf()
	}
	text := s.(string)
	location := re.FindStringIndex(text)
	if location == nil {
		return NoneOf()
	}
	return SomeOf(text[location[0]:location[1]])
}

func RegexFindAll(pattern, s any) any {
	re, err := regexp.Compile(pattern.(string))
	if err != nil {
		return ListFromStrings(nil)
	}
	return ListFromStrings(re.FindAllString(s.(string), -1))
}

// RegexReplace uses Go regexp replacement expansion ($1, ${name}, $$).
func RegexReplace(pattern, s, repl any) any {
	re, err := regexp.Compile(pattern.(string))
	if err != nil {
		return s
	}
	return re.ReplaceAllString(s.(string), repl.(string))
}

func RegexSplit(pattern, s any) any {
	re, err := regexp.Compile(pattern.(string))
	if err != nil {
		return ListFromStrings(nil)
	}
	return ListFromStrings(re.Split(s.(string), -1))
}

// RegexCaptures returns the first match's capture groups, excluding the whole match.
// A matching pattern without groups returns Some(Nil), not None.
func RegexCaptures(pattern, s any) any {
	re, err := regexp.Compile(pattern.(string))
	if err != nil {
		return NoneOf()
	}
	groups := re.FindStringSubmatch(s.(string))
	if groups == nil {
		return NoneOf()
	}
	return SomeOf(ListFromStrings(groups[1:]))
}
