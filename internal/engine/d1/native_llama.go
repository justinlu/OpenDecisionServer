//go:build llama && darwin && arm64 && cgo

package d1

/*
#include <stdlib.h>
#include <stdio.h>
#include <dlfcn.h>
#include "../../../native/d1/bridge.h"
struct ods_d1_api {
 void * library;
 __typeof__(&ods_d1_open) open;
 __typeof__(&ods_d1_run) run;
 __typeof__(&ods_d1_info) info;
 __typeof__(&ods_d1_close) close;
 __typeof__(&ods_d1_free) release;
 __typeof__(&ods_d1_abi) abi;
};
static struct ods_d1_api * ods_d1_load(const char * path,char * error,int size){
 struct ods_d1_api * a=calloc(1,sizeof(*a));if(!a)return NULL;
 a->library=dlopen(path,RTLD_NOW|RTLD_LOCAL);
 if(!a->library){snprintf(error,size,"d1 bridge load: %s",dlerror());free(a);return NULL;}
 #define LOAD(field,name) a->field=(__typeof__(a->field))dlsym(a->library,name); if(!a->field){snprintf(error,size,"missing d1 ABI symbol: %s",name);free(a);return NULL;}
 LOAD(open,"ods_d1_open") LOAD(run,"ods_d1_run") LOAD(info,"ods_d1_info") LOAD(close,"ods_d1_close") LOAD(release,"ods_d1_free") LOAD(abi,"ods_d1_abi")
 #undef LOAD
 if(a->abi()!=ODS_D1_ABI){snprintf(error,size,"d1 bridge ABI mismatch");free(a);return NULL;}
 return a; // Metal DSO remains loaded until process exit.
}
static void * ods_d1_create(struct ods_d1_api * a,const char * model,int threads,char ** error){return a->open(model,threads,error);}
static int ods_d1_call(struct ods_d1_api * a,void * h,const char * text,int budget,char ** out){return a->run(h,text,budget,out);}
static char * ods_d1_metadata(struct ods_d1_api * a,void * h){return a->info(h);}
static void ods_d1_release(struct ods_d1_api * a,char * text){a->release(text);}
static void ods_d1_destroy(struct ods_d1_api * a,void * h){a->close(h);free(a);}
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"github.com/opendecisionserver/ods/internal/decision"
	"strings"
	"sync/atomic"
	"unsafe"
)

var owned atomic.Bool

func nativeBuildError() error { return nil }

type llamaNative struct {
	api    *C.struct_ods_d1_api
	handle unsafe.Pointer
	info   map[string]any
}

func openNative(opts Options) (native, error) {
	if !owned.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("only one d1 runtime per process")
	}
	success := false
	defer func() {
		if !success {
			owned.Store(false)
		}
	}()
	lib := C.CString(opts.LibraryPath)
	defer C.free(unsafe.Pointer(lib))
	errbuf := make([]byte, 2048)
	api := C.ods_d1_load(lib, (*C.char)(unsafe.Pointer(&errbuf[0])), C.int(len(errbuf)))
	if api == nil {
		return nil, fmt.Errorf("%s", strings.TrimRight(string(errbuf), "\x00"))
	}
	model := C.CString(opts.ModelPath)
	defer C.free(unsafe.Pointer(model))
	var message *C.char
	handle := C.ods_d1_create(api, model, C.int(opts.Threads), &message)
	if handle == nil {
		text := "d1 initialization failed"
		if message != nil {
			text = C.GoString(message)
			C.ods_d1_release(api, message)
		}
		C.free(unsafe.Pointer(api))
		return nil, fmt.Errorf("%s", text)
	}
	n := &llamaNative{api: api, handle: handle}
	info := C.ods_d1_metadata(api, handle)
	if info == nil {
		n.Close()
		return nil, fmt.Errorf("d1 metadata failed")
	}
	err := json.Unmarshal([]byte(C.GoString(info)), &n.info)
	C.ods_d1_release(api, info)
	if err != nil {
		n.Close()
		return nil, err
	}
	n.info["model_sha256"] = ModelSHA256
	n.info["source_sha256"] = SourceSHA256
	n.info["patch_sha256"] = PatchSHA256
	success = true
	return n, nil
}
func (n *llamaNative) Run(payload []byte, budget int) ([]decision.Result, error) {
	request := C.CString(string(payload))
	defer C.free(unsafe.Pointer(request))
	var out *C.char
	rc := C.ods_d1_call(n.api, n.handle, request, C.int(budget), &out)
	if out == nil {
		return nil, fmt.Errorf("d1 allocation failed")
	}
	defer C.ods_d1_release(n.api, out)
	text := C.GoString(out)
	if rc == 1 {
		return nil, decision.FieldError{Location: []any{"body", "questions"}, Message: text, Type: "value_error"}
	}
	if rc != 0 {
		return nil, fmt.Errorf("d1 native: %s", text)
	}
	var raw []struct {
		Answer json.RawMessage `json:"answer"`
		Tokens int             `json:"input_tokens"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, err
	}
	results := make([]decision.Result, len(raw))
	for i, r := range raw {
		results[i] = decision.Result{Answer: r.Answer, InputTokens: r.Tokens}
	}
	return results, nil
}
func (n *llamaNative) Info() map[string]any { return n.info }
func (n *llamaNative) Close() error {
	C.ods_d1_destroy(n.api, n.handle)
	n.handle = nil
	owned.Store(false)
	return nil
}
