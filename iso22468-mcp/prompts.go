package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// Prompt Handlers

// promptArg returns the named prompt argument, or def when it is not set.
func promptArg(req mcp.GetPromptRequest, key, def string) string {
	if value, ok := req.Params.Arguments[key]; ok {
		return value
	}
	return def
}

// promptResult returns the system context followed by the user request.
func promptResult(description, systemMessage, userMessage string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Description: description,
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(systemMessage),
			},
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(userMessage),
			},
		},
	}
}

func handleVSMAnalysisPrompt(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	industry := promptArg(req, "industry", "general")
	complexity := promptArg(req, "complexity", "medium")

	systemMessage := fmt.Sprintf(`You are a Value Stream Mapping (VSM) expert helping with current state analysis according to ISO 22468:2020.

Industry Context: %s
Complexity Level: %s

Your role is to guide through a systematic current state analysis that identifies:
1. Current process flow and bottlenecks
2. Waste identification using the 8 types of muda
3. Data collection requirements
4. Key performance indicators calculation
5. Improvement opportunity identification

Always follow the structured approach:
- Start with customer requirements (takt time, demand)
- Map current processes with cycle times and yields
- Identify inventory points and quantities
- Calculate current KPIs (lead time, process efficiency, etc.)
- Identify bottlenecks and constraints
- Document all forms of waste
- Prioritize improvement opportunities`, industry, complexity)

	userMessage := `Please help me analyze the current state of my value stream. I need to:

1. **Define the Scope**: What product family and customer requirements should I start with?
2. **Map the Current State**: What processes, inventory points, and information flows need to be documented?
3. **Collect Data**: What timing and quality data should I gather for each process?
4. **Identify Waste**: How do I systematically identify the 8 types of waste (muda)?
5. **Find Bottlenecks**: What techniques help identify constraining processes?
6. **Calculate KPIs**: Which key performance indicators should I calculate and why?
7. **Document Findings**: How should I present the current state analysis results?

Please provide a step-by-step guide tailored to my industry context and complexity level.`

	return promptResult(
		fmt.Sprintf("Comprehensive guide for VSM current state analysis in %s industry (%s complexity)", industry, complexity),
		systemMessage, userMessage), nil
}

func handleVSMImplementationPrompt(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	timeline := promptArg(req, "timeline", "6 months")
	resources := promptArg(req, "resources", "limited")

	systemMessage := fmt.Sprintf(`You are a Lean implementation expert specializing in Value Stream Management transformation.

Implementation Context:
- Timeline: %s
- Resources: %s

Your expertise covers:
1. Future state design principles (8 Rother & Shook principles)
2. Implementation planning with SMART measures
3. Change management and stakeholder engagement
4. Risk assessment and mitigation
5. Progress monitoring and PDCA cycles
6. Sustainment of improvements

Guide implementations using proven lean techniques while considering resource constraints and timeline requirements.`, timeline, resources)

	userMessage := `Please help me plan and execute a VSM implementation. I need guidance on:

1. **Future State Design**: How do I apply the 8 design principles to create an improved value stream?
2. **SMART Measures**: What makes a good improvement measure (Specific, Measurable, Achievable, Relevant, Time-bound)?
3. **Implementation Planning**: How should I sequence and prioritize improvements?
4. **Risk Management**: What risks should I consider and how to mitigate them?
5. **Change Management**: How do I engage stakeholders and manage resistance?
6. **Progress Tracking**: What metrics should I track during implementation?
7. **Sustainment**: How do I ensure improvements are maintained long-term?

Please provide practical guidance considering my timeline and resource constraints.`

	return promptResult(
		fmt.Sprintf("VSM implementation guide for %s timeline with %s resources", timeline, resources),
		systemMessage, userMessage), nil
}
