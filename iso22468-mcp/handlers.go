package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	vsm "github.com/iso22468/vsm-go-sdk"
	"github.com/mark3labs/mcp-go/mcp"
)

// requireArg returns a required string argument, or the standard error result
// when it is missing.
func requireArg(req mcp.CallToolRequest, name string) (string, *mcp.CallToolResult) {
	value, err := req.RequireString(name)
	if err != nil {
		return "", mcp.NewToolResultError(fmt.Sprintf("Missing %s: %v", name, err))
	}
	return value, nil
}

// decodeArg unmarshals JSON text into a T. Invalid JSON yields the error result
// "<label>: <err>".
func decodeArg[T any](raw, label string) (T, *mcp.CallToolResult) {
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return v, mcp.NewToolResultError(fmt.Sprintf("%s: %v", label, err))
	}
	return v, nil
}

// jsonResult returns prefix followed by v as indented JSON.
func jsonResult(prefix string, v any) *mcp.CallToolResult {
	resultJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize result: %v", err))
	}
	return mcp.NewToolResultText(prefix + string(resultJSON))
}

// runAnalysis is the shared body of the analysis tools: it reads the value
// stream, runs one analyzer method and returns the result as JSON.
func runAnalysis[R any](req mcp.CallToolRequest, analyze func(*vsm.Analyzer, *vsm.ValueStream) (R, error), failure string, success func(R) string) (*mcp.CallToolResult, error) {
	valueStreamJSON, errResult := requireArg(req, "value_stream_json")
	if errResult != nil {
		return errResult, nil
	}

	vs, errResult := decodeArg[vsm.ValueStream](valueStreamJSON, "Invalid value stream JSON")
	if errResult != nil {
		return errResult, nil
	}

	result, err := analyze(vsm.NewAnalyzer(), &vs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("%s: %v", failure, err)), nil
	}

	return jsonResult(success(result), result), nil
}

// VSM Analysis Handlers

func handleVSMAnalysis(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runAnalysis(req, (*vsm.Analyzer).AnalyzeCurrentState, "Analysis failed",
		func(*vsm.AnalysisResult) string { return "Analysis completed successfully:\n" })
}

func handleBottleneckDetection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runAnalysis(req, (*vsm.Analyzer).IdentifyBottlenecks, "Bottleneck detection failed",
		func(bottlenecks []vsm.BottleneckInfo) string {
			return fmt.Sprintf("Found %d bottlenecks:\n", len(bottlenecks))
		})
}

func handleWasteAnalysis(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return runAnalysis(req, (*vsm.Analyzer).AnalyzeWaste, "Waste analysis failed",
		func(*vsm.WasteAnalysis) string { return "Waste analysis completed:\n" })
}

// VSM Design Handlers

func handleFutureStateDesign(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	currentStateJSON, errResult := requireArg(req, "current_state_json")
	if errResult != nil {
		return errResult, nil
	}

	analysisResultJSON, errResult := requireArg(req, "analysis_result_json")
	if errResult != nil {
		return errResult, nil
	}

	currentState, errResult := decodeArg[vsm.ValueStream](currentStateJSON, "Invalid current state JSON")
	if errResult != nil {
		return errResult, nil
	}

	analysisResult, errResult := decodeArg[vsm.AnalysisResult](analysisResultJSON, "Invalid analysis result JSON")
	if errResult != nil {
		return errResult, nil
	}

	designer := vsm.NewDesigner()
	designResult, err := designer.DesignFutureState(&currentState, &analysisResult)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Future state design failed: %v", err)), nil
	}

	return jsonResult("Future state design completed:\n", designResult), nil
}

func handleIdealStateGeneration(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	currentStateJSON, errResult := requireArg(req, "current_state_json")
	if errResult != nil {
		return errResult, nil
	}

	currentState, errResult := decodeArg[vsm.ValueStream](currentStateJSON, "Invalid current state JSON")
	if errResult != nil {
		return errResult, nil
	}

	designer := vsm.NewDesigner()
	idealState, err := designer.CreateIdealState(&currentState)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Ideal state generation failed: %v", err)), nil
	}

	return jsonResult("Ideal state generated:\n", idealState), nil
}

// VSM Planning Handlers

func handleImplementationPlanning(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	designResultJSON, errResult := requireArg(req, "design_result_json")
	if errResult != nil {
		return errResult, nil
	}

	projectJSON, errResult := requireArg(req, "project_json")
	if errResult != nil {
		return errResult, nil
	}

	designResult, errResult := decodeArg[vsm.DesignResult](designResultJSON, "Invalid design result JSON")
	if errResult != nil {
		return errResult, nil
	}

	project, errResult := decodeArg[vsm.VSMProject](projectJSON, "Invalid project JSON")
	if errResult != nil {
		return errResult, nil
	}

	planner := vsm.NewPlanner()
	planningResult, err := planner.PlanImplementation(&designResult, &project)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Implementation planning failed: %v", err)), nil
	}

	return jsonResult("Implementation plan created:\n", planningResult), nil
}

// VSM Assessment Handlers

func handlePerformanceAssessment(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	valueStreamJSON, errResult := requireArg(req, "value_stream_json")
	if errResult != nil {
		return errResult, nil
	}

	targetsJSON, errResult := requireArg(req, "targets_json")
	if errResult != nil {
		return errResult, nil
	}

	vs, errResult := decodeArg[vsm.ValueStream](valueStreamJSON, "Invalid value stream JSON")
	if errResult != nil {
		return errResult, nil
	}

	targets, errResult := decodeArg[map[string]float64](targetsJSON, "Invalid targets JSON")
	if errResult != nil {
		return errResult, nil
	}

	var history []vsm.AssessmentResult
	if historyJSON := req.GetString("history_json", ""); historyJSON != "" {
		history, errResult = decodeArg[[]vsm.AssessmentResult](historyJSON, "Invalid history JSON")
		if errResult != nil {
			return errResult, nil
		}
	}

	assessor := vsm.NewAssessor()
	assessment, err := assessor.AssessValueStream(&vs, targets, history)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Performance assessment failed: %v", err)), nil
	}

	return jsonResult("Performance assessment completed:\n", assessment), nil
}

// VSM Serialization Handlers

func handleValueStreamSerialization(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	valueStreamJSON, errResult := requireArg(req, "value_stream_json")
	if errResult != nil {
		return errResult, nil
	}

	prettyPrint := req.GetBool("pretty_print", false)

	vs, errResult := decodeArg[vsm.ValueStream](valueStreamJSON, "Invalid value stream JSON")
	if errResult != nil {
		return errResult, nil
	}

	serializer := vsm.NewSerializer()

	serializedData, err := serializer.SerializeValueStream(&vs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Serialization failed: %v", err)), nil
	}

	// Pretty output round-trips through interface{} on purpose: that makes
	// MarshalIndent sort the keys.
	if prettyPrint {
		var result interface{}
		if err := json.Unmarshal(serializedData, &result); err == nil {
			if prettyData, err := json.MarshalIndent(result, "", "  "); err == nil {
				serializedData = prettyData
			}
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Value stream serialized successfully (%d bytes):\n%s", len(serializedData), string(serializedData))), nil
}

func handleValueStreamDeserialization(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jsonData, errResult := requireArg(req, "json_data")
	if errResult != nil {
		return errResult, nil
	}

	serializer := vsm.NewSerializer()
	vs, err := serializer.DeserializeValueStream([]byte(jsonData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Deserialization failed: %v", err)), nil
	}

	return jsonResult("Value stream deserialized successfully:\n", vs), nil
}

func handleSaveToFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	valueStreamJSON, errResult := requireArg(req, "value_stream_json")
	if errResult != nil {
		return errResult, nil
	}

	filename, errResult := requireArg(req, "filename")
	if errResult != nil {
		return errResult, nil
	}

	vs, errResult := decodeArg[vsm.ValueStream](valueStreamJSON, "Invalid value stream JSON")
	if errResult != nil {
		return errResult, nil
	}

	serializer := vsm.NewSerializer()
	if err := serializer.SaveToFile(&vs, filename); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Save to file failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Value stream saved to file '%s' successfully", filename)), nil
}

func handleLoadFromFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filename, errResult := requireArg(req, "filename")
	if errResult != nil {
		return errResult, nil
	}

	serializer := vsm.NewSerializer()
	var vs vsm.ValueStream
	if err := serializer.LoadFromFile(filename, &vs); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Load from file failed: %v", err)), nil
	}

	return jsonResult(fmt.Sprintf("Value stream loaded from file '%s':\n", filename), vs), nil
}

// VSM Utility Handlers

func handleKPICalculation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	valueStreamJSON, errResult := requireArg(req, "value_stream_json")
	if errResult != nil {
		return errResult, nil
	}

	vs, errResult := decodeArg[vsm.ValueStream](valueStreamJSON, "Invalid value stream JSON")
	if errResult != nil {
		return errResult, nil
	}

	calculator := vsm.NewCalculator()
	kpis := calculator.CalculateKPIs(&vs)

	return jsonResult("KPIs calculated successfully:\n", kpis), nil
}

func handleCreateValueStream(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, errResult := requireArg(req, "id")
	if errResult != nil {
		return errResult, nil
	}

	name, errResult := requireArg(req, "name")
	if errResult != nil {
		return errResult, nil
	}

	productFamily, errResult := requireArg(req, "product_family")
	if errResult != nil {
		return errResult, nil
	}

	var customer *vsm.Customer
	if customerJSON := req.GetString("customer_json", ""); customerJSON != "" {
		customer, errResult = decodeArg[*vsm.Customer](customerJSON, "Invalid customer JSON")
		if errResult != nil {
			return errResult, nil
		}
	}

	vs := &vsm.ValueStream{
		ID:            id,
		Name:          name,
		State:         vsm.StateCurrent,
		ProductFamily: productFamily,
		Customer:      customer,
		Processes:     []vsm.Process{},
		Inventory:     []vsm.Inventory{},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       "1.0",
	}

	return jsonResult("Value stream created successfully:\n", vs), nil
}
