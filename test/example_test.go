package test

import (
	"testing"

	"github.com/catalystcommunity/semver-tags/core/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ExampleSuite struct {
	suite.Suite
}

// called before the entire suite is run
func (s *ExampleSuite) SetupSuite() {}

// called after the entire suite is run
func (s *ExampleSuite) TearDownSuite() {}

// called before each test
func (s *ExampleSuite) SetupTest() {}

// called after each test
func (s *ExampleSuite) TearDownTest() {}

// runs the entire suite
func TestExampleSuite(t *testing.T) {
	suite.Run(t, new(ExampleSuite))
}

func (s *ExampleSuite) TestBumpMajor() {
	version := semver.NewSemver(0, 1, 0)
	version.BumpMajor()

	assert.Equal(s.T(), version, semver.NewSemver(1, 0, 0), "bumpMajor did not give correct 1st bump result")
	version.BumpMajor()
	assert.Equal(s.T(), version, semver.NewSemver(2, 0, 0), "bumpMajor did not give correct 2nd bump result")
}
func (s *ExampleSuite) TestBumpMinor() {
	version := semver.NewSemver(0, 1, 0)
	version.BumpMinor()

	assert.Equal(s.T(), version, semver.NewSemver(0, 2, 0), "bumpMajor did not give correct 1st bump result")
	version.BumpMinor()
	assert.Equal(s.T(), version, semver.NewSemver(0, 3, 0), "bumpMajor did not give correct 2nd bump result")
}
func (s *ExampleSuite) TestBumpPatch() {
	version := semver.NewSemver(0, 1, 0)
	version.BumpPatch()

	assert.Equal(s.T(), version, semver.NewSemver(0, 1, 1), "bumpMajor did not give correct 1st bump result")
	version.BumpPatch()
	assert.Equal(s.T(), version, semver.NewSemver(0, 1, 2), "bumpMajor did not give correct 2nd bump result")
}
