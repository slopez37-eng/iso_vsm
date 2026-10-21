package vsm

import (
	"strings"
	"testing"
)

func TestReadCSVSemicolonProcesses(t *testing.T) {
	bom := "\xEF\xBB\xBF"
	in := bom + "Element; ID ;Name;Type;process_time;batch_size;times_to_repair;shared;value_adding_time\n" +
		"process;P1; cutting ;manual;2.5;;1|3;yes;1.5\n" +
		"process;P2;assembly;automated;4;10;;no;\n"
	vs := &ValueStream{}
	if err := ReadCSV(strings.NewReader(in), vs); err != nil {
		t.Fatal(err)
	}
	if len(vs.Processes) != 2 {
		t.Fatalf("got %d processes, want 2", len(vs.Processes))
	}
	p1, p2 := vs.Processes[0], vs.Processes[1]
	if p1.ID != "P1" || p1.Name != "cutting" || p1.Type != ProcessManual {
		t.Errorf("P1 = %q %q %q, want P1 cutting manual", p1.ID, p1.Name, p1.Type)
	}
	if p1.ProcessTime == nil || *p1.ProcessTime != 2.5 {
		t.Errorf("P1 process_time = %v, want 2.5", p1.ProcessTime)
	}
	if len(p1.TimesToRepair) != 2 || p1.TimesToRepair[0] != 1 || p1.TimesToRepair[1] != 3 {
		t.Errorf("P1 times_to_repair = %v, want [1 3]", p1.TimesToRepair)
	}
	if !p1.Shared || p1.BatchSize != nil {
		t.Errorf("P1 shared = %v, batch_size = %v, want true and not entered", p1.Shared, p1.BatchSize)
	}
	if p1.ValueAdding == nil || *p1.ValueAdding != 1.5 {
		t.Errorf("P1 value_adding_time = %v, want 1.5", p1.ValueAdding)
	}
	if p2.Shared || p2.BatchSize == nil || *p2.BatchSize != 10 || p2.ValueAdding != nil {
		t.Errorf("P2 = %+v, want not shared, batch 10 and no value adding time", p2)
	}
}

func TestReadCSVIdleWithInventories(t *testing.T) {
	in := strings.Join([]string{
		"element,id,name,process_time,from,to,idle_time,inventory_kind,inventory_quantity,inventory_range",
		"process,P1,cutting,2,,,,,,",
		"idle,I1,,,,,3,stock,100,",
		"process,P2,assembly,4,,,,,,",
		"inventory,,,,,,,warehouse,50,",
		"idle,I2,,,,,5,,,",
	}, "\n")
	vs := &ValueStream{}
	if err := ReadCSV(strings.NewReader(in), vs); err != nil {
		t.Fatal(err)
	}
	if len(vs.Idles) != 2 {
		t.Fatalf("got %d idles, want 2", len(vs.Idles))
	}
	i1, i2 := vs.Idles[0], vs.Idles[1]
	if i1.From != "P1" || i1.To != "P2" {
		t.Errorf("I1 from %q to %q, want P1 and P2", i1.From, i1.To)
	}
	if len(i1.Inventories) != 2 || i1.Inventories[0].Kind != InventoryStock || i1.Inventories[1].Kind != InventoryWarehouse {
		t.Fatalf("I1 inventories = %+v, want stock then warehouse", i1.Inventories)
	}
	if q := i1.Inventories[1].Quantity; q == nil || *q != 50 {
		t.Errorf("warehouse quantity = %v, want 50", q)
	}
	if i2.From != "P2" || i2.To != "" {
		t.Errorf("I2 from %q to %q, want P2 and the customer (empty)", i2.From, i2.To)
	}
}

func TestReadCSVRejectsBadInput(t *testing.T) {
	for _, c := range []struct {
		name, in, want string
	}{
		{"unknown column", "element,id,colour\nprocess,P1,red\n", `unknown column "colour"`},
		{"value in a column the element lacks", "element,id,setup_time\nprocess,P1,1\nidle,I1,2\n", "CSV line 3, column setup_time"},
		{"decimal comma", "element;id;process_time\nprocess;P1;1,5\n", "use a point"},
		{"bad number", "element,id,process_time\nprocess,P1,fast\n", `"fast" is not a number`},
		{"bad boolean", "element,id,shared\nprocess,P1,maybe\n", `"maybe" is not true, false`},
		{"inventory before an idle", "element,id,inventory_kind\ninventory,,stock\n", "needs an idle row before it"},
		{"no element column", "id,name\nP1,x\n", "no element column"},
		{"unknown element", "element,id\nstep,P1\n", `element "step" is not process, idle or inventory`},
		{"repeated column", "element,id,ID\n", `"id" appears twice`},
		{"ragged row", "element,id,process_time\nprocess,P1\n", "record on line 2"},
	} {
		vs := &ValueStream{}
		err := ReadCSV(strings.NewReader(c.in), vs)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", c.name, err, c.want)
		}
		if len(vs.Processes)+len(vs.Idles) != 0 {
			t.Errorf("%s: a failed import changed the stream", c.name)
		}
	}
}
