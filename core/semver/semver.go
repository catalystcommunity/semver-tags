package semver

import (
	"fmt"
	"strconv"
	"strings"
)

type CommitType int

const (
	NotConventional CommitType = iota
	Patch
	Minor
	Major
)

type Semver struct {
	Major      uint32
	Minor      uint32
	Patch      uint32
	PreRelease string
	Build      string
}

func NewSemver(major, minor, patch uint32) *Semver {
	return &Semver{Major: major, Minor: minor, Patch: patch}
}

func (v *Semver) Clone() *Semver {
	retVal := NewSemver(v.Major, v.Minor, v.Patch)
	retVal.PreRelease = v.PreRelease
	retVal.Build = v.Build
	return retVal
}

// Compare gives -1 when v is lower than other, 0 when the two are the same,
// and 1 when v is higher. It uses semantic version precedence. It ignores the
// build part, and it puts a pre-release below the related release.
func (v *Semver) Compare(other *Semver) int {
	if result := compareNumbers(v.Major, other.Major); result != 0 {
		return result
	}
	if result := compareNumbers(v.Minor, other.Minor); result != 0 {
		return result
	}
	if result := compareNumbers(v.Patch, other.Patch); result != 0 {
		return result
	}
	return comparePreRelease(v.PreRelease, other.PreRelease)
}

func compareNumbers(left uint32, right uint32) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// comparePreRelease compares two pre-release parts. An empty pre-release is
// higher than any pre-release, because 1.0.0 comes after 1.0.0-rc.1.
func comparePreRelease(left string, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}

	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		if result := compareIdentifiers(leftParts[index], rightParts[index]); result != 0 {
			return result
		}
	}
	return compareNumbers(uint32(len(leftParts)), uint32(len(rightParts)))
}

// compareIdentifiers compares one part of a pre-release. A part that holds
// only digits is lower than a part that holds text.
func compareIdentifiers(left string, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	switch {
	case leftErr == nil && rightErr == nil:
		return compareNumbers(uint32(leftNumber), uint32(rightNumber))
	case leftErr == nil:
		return -1
	case rightErr == nil:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

// BumpVersion applies a commit level and optional version identifiers.
func (v *Semver) BumpVersion(commitType CommitType, preRelease string, build string) {
	cleanPreRelease := strings.Trim(preRelease, " \n\r\t")
	currentPreRelease := strings.Split(v.PreRelease, ".")[0]

	if currentPreRelease != "" && cleanPreRelease == currentPreRelease {
		v.IncrementPreRelease()
		return
	}
	if cleanPreRelease != "" {
		v.PreRelease = preRelease + ".1"
		if build != "" {
			v.Build = build
		}
		return
	}

	switch commitType {
	case Patch:
		v.BumpPatch()
	case Minor:
		v.BumpMinor()
	case Major:
		v.BumpMajor()
	case NotConventional:
		return
	default:
		return
	}
	if build != "" {
		v.Build = build
	}
}

func (v *Semver) BumpMajor() {
	v.Major += 1
	v.Minor = 0
	v.Patch = 0
	v.PreRelease = ""
	v.Build = ""
}

func (v *Semver) BumpMinor() {
	v.Minor += 1
	v.Patch = 0
	v.PreRelease = ""
	v.Build = ""
}

func (v *Semver) BumpPatch() {
	v.Patch += 1
	v.PreRelease = ""
	v.Build = ""
}

func (v *Semver) IncrementPreRelease() {
	parts := strings.Split(v.PreRelease, ".")
	if len(parts) < 2 {
		v.PreRelease = parts[0] + ".2"
		return
	}

	numberPart := parts[1]
	number, err := strconv.Atoi(numberPart)
	// Restart at 1 when the existing suffix is not a number.
	if err != nil {
		v.PreRelease = parts[0] + ".1"
		return
	}

	v.PreRelease = parts[0] + "." + fmt.Sprintf("%d", number+1)
}

func (v *Semver) FormattedString() string {
	retVal := "v"

	retVal += fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)

	if v.PreRelease != "" {
		retVal += fmt.Sprintf("-%s", v.PreRelease)
	}
	if v.Build != "" {
		retVal += fmt.Sprintf("+%s", v.Build)
	}

	return retVal
}
