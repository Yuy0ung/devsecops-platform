package sast

import (
	"context"
	"demo/models"
	"demo/pkg/aiaudit"
	"log"
	"path/filepath"
)

// performAIAudit handles the integration between SAST findings and the AI audit service.
// It modifies the finding in-place if the AI determines it's a false positive.
func performAIAudit(task models.SastTask, workDir string, ruleID string, codeFlows []CodeFlowLocation, finding *models.SastFinding) {
	// Check if AI audit is enabled
	if !task.EnableAIAudit {
		return
	}

	// Only audit if there is a code flow (context)
	if len(codeFlows) == 0 {
		return
	}

	log.Printf("Starting AI audit for finding: %s", ruleID)

	// Convert local code flows to audit format
	var auditFlows []aiaudit.CodeLocation

	// Note: CodeQL artifacts URIs are relative to source root, we need absolute paths
	// The "source" directory is at filepath.Join(workDir, "source")
	sourceRoot := filepath.Join(workDir, "source")
	if task.Type != "Git" {
		sourceRoot = workDir // For upload mode, it's just workDir
	}

	for _, cf := range codeFlows {
		// Resolve absolute path
		// CodeQL URIs might be relative, e.g. "src/main/java/..."
		absPath := filepath.Join(sourceRoot, cf.File)
		auditFlows = append(auditFlows, aiaudit.CodeLocation{
			FilePath: absPath,
			Line:     cf.Line,
		})
	}

	auditState := &aiaudit.AuditState{
		VulnerabilityID: ruleID, // simplified
		RuleID:          ruleID,
		VulnDescription: finding.Description,
		CodeFlows:       auditFlows,
	}

	// Run the pipeline (context extraction + AI check)
	// Use a background context for now, but could be a timeout context
	if err := aiaudit.RunAuditPipeline(context.Background(), auditState); err != nil {
		log.Printf("AI Audit failed for rule %s: %v", ruleID, err)
		return
	}

	// Apply results to the finding
	finding.AIAnalysis = auditState.Thinking // Save the thinking process log

	if auditState.IsFalsePositive {
		finding.Severity = "Info" // Downgrade severity
		finding.Description = "[AI: Suspected False Positive] " + finding.Description + "\nReason: " + auditState.Reason
		// Also update Message as some frontend views might use it
		finding.Message = finding.Description
	} else {
		// For verified true positives, we don't change the description significantly, or maybe just append a confirmation
		// finding.Description = finding.Description + "\n[AI: Verified]"
	}
}
