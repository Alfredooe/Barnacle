package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAffectedStacks(t *testing.T) {
	testCases := []struct {
		name             string
		changedFiles     []string
		currentStacks    map[string]bool
		deployedStacks   map[string]bool
		repoPath         string
		expectedAffected map[string]bool
		expectedDeleted  []string
	}{
		{
			name:             "Normal change",
			changedFiles:     []string{"stack1/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{"stack1": true},
			expectedDeleted:  []string{},
		},
		{
			name:             "Change in subdirectory",
			changedFiles:     []string{"stack1/subdir/config.json"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{"stack1": true},
			expectedDeleted:  []string{},
		},
		{
			name:             "Root file change",
			changedFiles:     []string{"README.md"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{},
			expectedDeleted:  []string{},
		},
		{
			name:             "Path traversal attempt",
			changedFiles:     []string{"../stack1/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{},
			expectedDeleted:  []string{},
		},
		{
			name:             "Path traversal with dot",
			changedFiles:     []string{"./stack1/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{"stack1": true},
			expectedDeleted:  []string{},
		},
		{
			name:             "New stack added",
			changedFiles:     []string{"stack3/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true, "stack3": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{"stack3": true},
			expectedDeleted:  []string{},
		},
		{
			name:             "Stack deleted",
			changedFiles:     []string{},
			currentStacks:    map[string]bool{"stack1": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "",
			expectedAffected: map[string]bool{},
			expectedDeleted:  []string{"stack2"},
		},
		{
			name:             "Complex changes",
			changedFiles:     []string{"stack1/docker-compose.yml", "stack4/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack3": true, "stack4": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true, "stack3": true},
			repoPath:         "",
			expectedAffected: map[string]bool{"stack1": true, "stack4": true},
			expectedDeleted:  []string{"stack2"},
		},
		{
			name:             "With repoPath - change inside subfolder",
			changedFiles:     []string{"stacks/stack1/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "stacks",
			expectedAffected: map[string]bool{"stack1": true},
			expectedDeleted:  []string{},
		},
		{
			name:             "With repoPath - change outside subfolder ignored",
			changedFiles:     []string{"other/stack1/docker-compose.yml", "README.md"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "stacks",
			expectedAffected: map[string]bool{},
			expectedDeleted:  []string{},
		},
		{
			name:             "With nested repoPath",
			changedFiles:     []string{"deploy/production/stack1/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true},
			deployedStacks:   map[string]bool{"stack1": true},
			repoPath:         "deploy/production",
			expectedAffected: map[string]bool{"stack1": true},
			expectedDeleted:  []string{},
		},
		{
			name:             "With repoPath - new stack detected",
			changedFiles:     []string{"stacks/stack3/docker-compose.yml"},
			currentStacks:    map[string]bool{"stack1": true, "stack2": true, "stack3": true},
			deployedStacks:   map[string]bool{"stack1": true, "stack2": true},
			repoPath:         "stacks",
			expectedAffected: map[string]bool{"stack3": true},
			expectedDeleted:  []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			affected, deleted := getAffectedStacks(tc.changedFiles, tc.currentStacks, tc.deployedStacks, tc.repoPath)
			assert.Equal(t, tc.expectedAffected, affected)
			assert.ElementsMatch(t, tc.expectedDeleted, deleted)
		})
	}
}
