package contractcandidate

import (
  "encoding/json"
  "os"
  "path/filepath"
  "testing"
)
func TestCandidateMASContractIsFailClosed(t *testing.T) {
 data,err:=os.ReadFile("v0.1.0/source-map.json")
 if err!=nil { t.Fatal(err) }
 var m struct {
  Kind string `json:"kind"`
  Status string `json:"status"`
  Deployable bool `json:"deployable"`
  Owner struct{OwnerID *string `json:"ownerId"`; MutationReceiptProducer bool `json:"mutationReceiptProducer"`; ReconcileProducer bool `json:"reconcileProducer"`} `json:"owner"`
  Operations []struct{
    StepID string `json:"stepId"`
    MAS string `json:"mas"`
    Op string `json:"op"`
    Capability string `json:"capabilityCandidate"`
    InputSchema string `json:"inputSchema"`
    OutputSchema string `json:"outputSchema"`
    AuthorInputs []string `json:"authorInputs"`
    SourceOutputFields []string `json:"sourceOutputFields"`
    Effect string `json:"effectCandidate"`
    Receipt string `json:"receiptPublication"`
    Mappings []struct{From string `json:"from"`; To any `json:"to"`} `json:"argumentMapping"`
  } `json:"operations"`
 }
 if err:=json.Unmarshal(data,&m);err!=nil {t.Fatal(err)}
 if m.Kind!="MASOwnerSourceContractCandidate" || m.Status!="NOT_QUALIFIED" || m.Deployable || m.Owner.OwnerID!=nil || m.Owner.MutationReceiptProducer || m.Owner.ReconcileProducer {t.Fatal("candidate incorrectly claims Owner publication")}
 expected:=map[string]string{"s1":"SC.VALIDATE_SUBSCRIPTION","s2":"SAA.SECONDARY_AUTH","s3":"SC.CREATE_SM_CONTEXT","s3r":"SC.REGISTER_UECM","s3p":"PA.CREATE_SM_POLICY","s4":"UPS.DECIDE_ANCHOR","s5":"IAM.ALLOCATE_ADDRESS","s6u":"PA.UPDATE_SM_POLICY","s7":"CA.CREATE_CHARGING","s8":"UPN.CREATE_UP_SESSION","s10":"SSM.SIGNAL_UE_AN","s11":"UPN.MODIFY_UP_SESSION","s13":"SE.EMIT_EVENTS"}
 if len(m.Operations)!=len(expected) {t.Fatalf("got %d operations, want 13",len(m.Operations))}
 seen:=map[string]bool{}
 for _,op:=range m.Operations {
  want,ok:=expected[op.StepID]; if !ok || seen[op.StepID] || op.MAS+"."+op.Op!=want {t.Fatalf("incorrect/duplicate operation %+v",op)}
  seen[op.StepID]=true
  if op.Effect!="MUTATION_REVIEW_REQUIRED" || op.Receipt!="ABSENT" || op.Capability=="" {t.Fatalf("unqualified effect or capability published: %s",op.StepID)}
  if len(op.AuthorInputs)!=len(op.Mappings)||len(op.SourceOutputFields)==0 {t.Fatalf("missing mappings or outputs: %s",op.StepID)}
  for _,item:=range []struct{file,id string; required []string}{
   {filepath.Join("v0.1.0","schemas",op.StepID+".input.schema.json"),op.InputSchema,op.AuthorInputs},
   {filepath.Join("v0.1.0","schemas",schemaFilename(op.OutputSchema)+".output.schema.json"),op.OutputSchema,op.SourceOutputFields},
  }{
   b,err:=os.ReadFile(item.file);if err!=nil{t.Fatal(err)}
   var schema struct{ID string `json:"$id"`;Type string `json:"type"`;AdditionalProperties bool `json:"additionalProperties"`; Required []string `json:"required"`; Properties map[string]json.RawMessage `json:"properties"`}
   if err:=json.Unmarshal(b,&schema);err!=nil{t.Fatalf("%s: %v",item.file,err)}
   if schema.ID!=item.id || schema.Type!="object"||schema.AdditionalProperties||len(schema.Required)!=len(item.required){t.Fatalf("invalid schema: %s",item.file)}
   for i,k:=range item.required {if schema.Required[i]!=k || len(schema.Properties[k])==0{t.Fatalf("missing expected field %s in %s",k,item.file)}}
  }
 }
}
func schemaFilename(id string)string{
 const prefix="urn:example:openamsm:mas:"
 s:=id[len(prefix):]
 return s[:len(s)-len(":v1")]
}
