package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

const (
	pollInterval  = 30 * time.Second
	stateFile     = "/app/barnacle-state.json"
	deployKeyPath = "/ssh/deploy_key"
)

type Config struct {
	RepoURL        string
	RepoPath       string
	Branch         string
	DiscordWebhook string
}

type State struct {
	DeployedStacks map[string]bool `json:"deployed_stacks"`
	LastCommit     string          `json:"last_commit"`
}

type DiscordWebhook struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

func main() {
	config := loadConfig()

	log.Printf("Starting barnacle...")
	log.Printf("Repository: %s", config.RepoURL)
	log.Printf("Local path: %s", config.RepoPath)
	log.Printf("Poll interval: %v", pollInterval)

	state := loadState()

	repo, err := initializeRepo(config)
	if err != nil {
		log.Fatalf("Failed to initialize repository: %v", err)
	}

	if repo != nil {
		if err := deployAllStacks(config.RepoPath, state); err != nil {
			log.Printf("Warning: Initial deployment failed: %v", err)
		}
	} else {
		log.Println("Skipping initial deployment, waiting for repository content...")
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Checking for updates...")

		if repo == nil {
			repo, err = initializeRepo(config)
			if err != nil {
				log.Printf("Error initializing repository: %v", err)
				continue
			}
			if repo == nil {
				continue
			}
			log.Println("Repository now has content, performing initial deployment...")
			if err := deployAllStacks(config.RepoPath, state); err != nil {
				log.Printf("Error deploying stacks: %v", err)
			}
			continue
		}

		updated, changedFiles, err := pullRepo(repo, config)
		if err != nil {
			log.Printf("Error pulling repository: %v", err)
			continue
		}

		if updated {
			log.Println("Repository updated, deploying changed stacks...")

			sendUpdateDetectedWebhook(config.DiscordWebhook, changedFiles)

			deploymentResults := make(map[string]error)
			if err := deployChanges(config.RepoPath, changedFiles, state, deploymentResults); err != nil {
				log.Printf("Error deploying stacks: %v", err)
			}

			sendDeploymentResultWebhook(config.DiscordWebhook, deploymentResults, changedFiles)
		} else {
			log.Println("No updates found")
		}
	}
}

func loadConfig() Config {
	repoURL := getEnv("REPO_URL", "")
	if repoURL == "" {
		log.Fatal("REPO_URL environment variable is required")
	}

	repoName := extractRepoName(repoURL)
	repoPath := getEnv("REPO_PATH", fmt.Sprintf("/opt/%s", repoName))

	config := Config{
		RepoURL:        repoURL,
		RepoPath:       repoPath,
		Branch:         getEnv("BRANCH", "main"),
		DiscordWebhook: getEnv("DISCORD_WEBHOOK", ""),
	}

	return config
}

func extractRepoName(repoURL string) string {
	repoURL = strings.TrimSuffix(repoURL, ".git")

	if strings.Contains(repoURL, ":") && strings.Contains(repoURL, "@") {
		parts := strings.Split(repoURL, ":")
		if len(parts) >= 2 {
			path := parts[len(parts)-1]
			return filepath.Base(path)
		}
	}

	return filepath.Base(repoURL)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func loadState() *State {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return &State{
			DeployedStacks: make(map[string]bool),
		}
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("Warning: Failed to load state file, creating new state: %v", err)
		return &State{
			DeployedStacks: make(map[string]bool),
		}
	}

	return &state
}

func saveState(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

func initializeRepo(config Config) (*git.Repository, error) {
	repo, err := git.PlainOpen(config.RepoPath)
	if err == nil {
		log.Println("Repository already exists, using existing clone")
		return repo, nil
	}

	log.Println("Cloning repository...")
	auth, err := getSSHAuth(deployKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to setup SSH auth: %w", err)
	}

	repo, err = git.PlainClone(config.RepoPath, false, &git.CloneOptions{
		URL:           config.RepoURL,
		Auth:          auth,
		ReferenceName: plumbing.NewBranchReferenceName(config.Branch),
		Progress:      os.Stdout,
	})
	if err != nil {
		if strings.Contains(err.Error(), "remote repository is empty") {
			log.Println("Repository is empty, will wait for content to be pushed")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to clone: %w", err)
	}

	log.Println("Repository cloned successfully")
	return repo, nil
}

func pullRepo(repo *git.Repository, config Config) (bool, []string, error) {
	w, err := repo.Worktree()
	if err != nil {
		return false, nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	headBefore, err := repo.Head()
	if err != nil {
		return false, nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	err = w.Reset(&git.ResetOptions{
		Mode: git.HardReset,
	})
	if err != nil {
		return false, nil, fmt.Errorf("failed to reset worktree: %w", err)
	}

	auth, err := getSSHAuth(deployKeyPath)
	if err != nil {
		return false, nil, fmt.Errorf("failed to setup SSH auth: %w", err)
	}

	err = w.Pull(&git.PullOptions{
		Auth:          auth,
		ReferenceName: plumbing.NewBranchReferenceName(config.Branch),
	})

	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to pull: %w", err)
	}

	headAfter, err := repo.Head()
	if err != nil {
		return false, nil, fmt.Errorf("failed to get HEAD after pull: %w", err)
	}

	updated := headBefore.Hash() != headAfter.Hash()
	if !updated {
		return false, nil, nil
	}

	log.Printf("Updated from %s to %s", headBefore.Hash().String()[:7], headAfter.Hash().String()[:7])

	changedFiles, err := getChangedFiles(repo, headBefore.Hash(), headAfter.Hash())
	if err != nil {
		log.Printf("Warning: Failed to get changed files, will deploy all stacks: %v", err)
		return true, nil, nil
	}

	return true, changedFiles, nil
}

func getChangedFiles(repo *git.Repository, oldCommit, newCommit plumbing.Hash) ([]string, error) {
	commitOld, err := repo.CommitObject(oldCommit)
	if err != nil {
		return nil, err
	}

	commitNew, err := repo.CommitObject(newCommit)
	if err != nil {
		return nil, err
	}

	treeOld, err := commitOld.Tree()
	if err != nil {
		return nil, err
	}

	treeNew, err := commitNew.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := treeOld.Diff(treeNew)
	if err != nil {
		return nil, err
	}

	var changedFiles []string
	for _, change := range changes {
		if change.To.Name != "" {
			changedFiles = append(changedFiles, change.To.Name)
		} else if change.From.Name != "" {
			changedFiles = append(changedFiles, change.From.Name)
		}
	}

	return changedFiles, nil
}

func getSSHAuth(keyPath string) (*ssh.PublicKeys, error) {
	auth, err := ssh.NewPublicKeysFromFile("git", keyPath, "")
	if err != nil {
		return nil, err
	}

	return auth, nil
}

func deployAllStacks(repoPath string, state *State) error {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return fmt.Errorf("failed to read repo directory: %w", err)
	}

	currentStacks := make(map[string]bool)
	deployedCount := 0

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}

		stackName := entry.Name()
		stackPath := filepath.Join(repoPath, stackName)

		if _, err := os.Stat(filepath.Join(stackPath, "ignore")); err == nil {
			log.Printf("Skipping %s: ignore file present", stackName)
			continue
		}

		if !hasComposeFile(stackPath) {
			log.Printf("Skipping %s: no compose file found", stackName)
			continue
		}

		currentStacks[stackName] = true

		log.Printf("Deploying stack: %s", stackName)
		if err := dockerComposeUp(stackPath); err != nil {
			log.Printf("Failed to deploy stack %s: %v", stackName, err)
			continue
		}

		deployedCount++
		log.Printf("Successfully deployed stack: %s", stackName)
	}

	deletedStacks := []string{}
	for stackName := range state.DeployedStacks {
		if !currentStacks[stackName] {
			deletedStacks = append(deletedStacks, stackName)
		}
	}
	cleanupDeletedStacks(repoPath, deletedStacks, make(map[string]error))

	state.DeployedStacks = currentStacks
	if err := saveState(state); err != nil {
		log.Printf("Warning: Failed to save state: %v", err)
	}

	log.Printf("Deployment complete: %d stack(s) deployed", deployedCount)
	return nil
}

func hasComposeFile(stackPath string) bool {
	for _, filename := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if _, err := os.Stat(filepath.Join(stackPath, filename)); err == nil {
			return true
		}
	}
	return false
}

func dockerComposeUp(stackPath string) error {
	cmd := exec.Command("docker", "compose", "up", "-d", "--remove-orphans")
	cmd.Dir = stackPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	return nil
}

func dockerComposeDown(stackPath string, projectName string) error {
	if _, err := os.Stat(stackPath); err == nil {
		cmd := exec.Command("docker", "compose", "down", "--remove-orphans")
		cmd.Dir = stackPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	cmd := exec.Command("docker", "compose", "-p", projectName, "down", "--remove-orphans")
	cmd.Dir = "/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func deployChanges(repoPath string, changedFiles []string, state *State, results map[string]error) error {
	if changedFiles == nil {
		return deployAllStacks(repoPath, state)
	}

	currentStacks, err := getCurrentStacks(repoPath)
	if err != nil {
		return err
	}

	affectedStacks, deletedStacks := getAffectedStacks(changedFiles, currentStacks, state.DeployedStacks)

	deployStacks(repoPath, affectedStacks, results)
	cleanupDeletedStacks(repoPath, deletedStacks, results)

	state.DeployedStacks = currentStacks
	if err := saveState(state); err != nil {
		log.Printf("Warning: Failed to save state: %v", err)
	}

	log.Printf("Deployment complete: %d stack(s) deployed", len(affectedStacks))
	return nil
}

func getCurrentStacks(repoPath string) (map[string]bool, error) {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read repo directory: %w", err)
	}

	currentStacks := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}

		stackName := entry.Name()
		stackPath := filepath.Join(repoPath, stackName)

		if _, err := os.Stat(filepath.Join(stackPath, "ignore")); err == nil {
			continue
		}

		if !hasComposeFile(stackPath) {
			continue
		}

		currentStacks[stackName] = true
	}

	log.Printf("Current stacks on disk: %v", mapKeys(currentStacks))
	return currentStacks, nil
}

func getAffectedStacks(changedFiles []string, currentStacks, deployedStacks map[string]bool) (map[string]bool, []string) {
	affectedStacks := make(map[string]bool)
	for _, file := range changedFiles {
		parts := strings.Split(file, "/")
		if len(parts) > 0 {
			stackName := parts[0]
			if stackName == ".." {
				log.Printf("    ✗ Skipping potentially malicious path: %s", file)
				continue
			}
			if stackName == "." {
				if len(parts) > 1 {
					stackName = parts[1]
				} else {
					continue
				}
			}
			if currentStacks[stackName] {
				affectedStacks[stackName] = true
			}
		}
	}

	for stackName := range currentStacks {
		if !deployedStacks[stackName] {
			affectedStacks[stackName] = true
			log.Printf("New stack detected: %s", stackName)
		}
	}

	deletedStacks := []string{}
	for stackName := range deployedStacks {
		if !currentStacks[stackName] {
			log.Printf("Deleted stack detected: %s", stackName)
			deletedStacks = append(deletedStacks, stackName)
		}
	}

	log.Printf("Affected stacks: %v", mapKeys(affectedStacks))
	return affectedStacks, deletedStacks
}

func deployStacks(repoPath string, affectedStacks map[string]bool, results map[string]error) {
	for stackName := range affectedStacks {
		stackPath := filepath.Join(repoPath, stackName)

		log.Printf("Deploying stack: %s", stackName)
		if err := dockerComposeUp(stackPath); err != nil {
			log.Printf("Failed to deploy stack %s: %v", stackName, err)
			results[stackName] = err
			continue
		}

		results[stackName] = nil
		log.Printf("Successfully deployed stack: %s", stackName)
	}
}

func cleanupDeletedStacks(repoPath string, deletedStacks []string, results map[string]error) {
	for _, stackName := range deletedStacks {
		stackPath := filepath.Join(repoPath, stackName)
		log.Printf("Stack %s was deleted, running docker compose down...", stackName)

		if err := dockerComposeDown(stackPath, stackName); err != nil {
			log.Printf("Warning: Failed to stop deleted stack %s: %v", stackName, err)
			results[stackName+" (deleted)"] = err
		} else {
			log.Printf("Successfully stopped deleted stack: %s", stackName)
			results[stackName+" (deleted)"] = nil
		}
	}
}

func sendUpdateDetectedWebhook(webhookURL string, changedFiles []string) {
	if webhookURL == "" {
		return
	}

	filesText := strings.Join(changedFiles, "\n")
	if len(filesText) > 1000 {
		filesText = filesText[:997] + "..."
	}

	embed := DiscordEmbed{
		Title:       "🔄 Update Detected",
		Description: "New changes detected in repository",
		Color:       3447003,
		Fields: []DiscordEmbedField{
			{
				Name:  "Changed Files",
				Value: "```\n" + filesText + "\n```",
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	webhook := DiscordWebhook{
		Embeds: []DiscordEmbed{embed},
	}

	sendDiscordWebhook(webhookURL, webhook)
}

func sendDeploymentResultWebhook(webhookURL string, results map[string]error, changedFiles []string) {
	if webhookURL == "" {
		return
	}

	successStacks := []string{}
	failedStacks := []string{}

	for stackName, err := range results {
		if err == nil {
			successStacks = append(successStacks, stackName)
		} else {
			failedStacks = append(failedStacks, fmt.Sprintf("%s: %v", stackName, err))
		}
	}

	var title string
	var color int
	var description string

	if len(failedStacks) == 0 {
		title = "✅ Deployment Successful"
		color = 3066993
		description = "All stacks deployed successfully"
	} else if len(successStacks) == 0 {
		title = "❌ Deployment Failed"
		color = 15158332
		description = "All stacks failed to deploy"
	} else {
		title = "⚠️ Deployment Partially Successful"
		color = 16776960
		description = "Some stacks failed to deploy"
	}

	fields := []DiscordEmbedField{}

	if len(successStacks) > 0 {
		successText := strings.Join(successStacks, "\n")
		if len(successText) > 1000 {
			successText = successText[:997] + "..."
		}
		fields = append(fields, DiscordEmbedField{
			Name:  fmt.Sprintf("✅ Success (%d)", len(successStacks)),
			Value: "```\n" + successText + "\n```",
		})
	}

	if len(failedStacks) > 0 {
		failedText := strings.Join(failedStacks, "\n")
		if len(failedText) > 1000 {
			failedText = failedText[:997] + "..."
		}
		fields = append(fields, DiscordEmbedField{
			Name:  fmt.Sprintf("❌ Failed (%d)", len(failedStacks)),
			Value: "```\n" + failedText + "\n```",
		})
	}

	embed := DiscordEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Fields:      fields,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	webhook := DiscordWebhook{
		Embeds: []DiscordEmbed{embed},
	}

	sendDiscordWebhook(webhookURL, webhook)
}

func sendDiscordWebhook(webhookURL string, webhook DiscordWebhook) {
	if webhookURL == "" {
		return
	}

	jsonData, err := json.Marshal(webhook)
	if err != nil {
		log.Printf("Failed to marshal Discord webhook: %v", err)
		return
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to send Discord webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("Discord webhook returned non-2xx status: %d", resp.StatusCode)
	} else {
		log.Println("Discord webhook sent successfully")
	}
}
