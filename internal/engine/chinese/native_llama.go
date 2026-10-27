//go:build llama && darwin && arm64 && cgo

package chinese

/*
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <dlfcn.h>
#include "llama.h"

#define FUNCTIONS(F) \
 F(llama_backend_init) F(llama_backend_free) F(llama_supports_gpu_offload) \
 F(llama_model_default_params) F(llama_context_default_params) \
 F(llama_model_load_from_file) F(llama_model_free) F(llama_model_get_vocab) \
 F(llama_init_from_model) F(llama_free) F(llama_tokenize) \
 F(llama_get_memory) F(llama_memory_clear) F(llama_batch_init) F(llama_batch_free) \
 F(llama_decode) F(llama_get_logits_ith) F(llama_synchronize) \
 F(llama_n_ctx) F(llama_n_ctx_seq) F(llama_n_batch) F(llama_n_ubatch) \
 F(ggml_backend_dev_by_type) F(ggml_backend_dev_description) F(ggml_backend_dev_memory)

struct ods_llama {
 void * library;
 struct llama_model * model;
 struct llama_context * ctx;
 const struct llama_vocab * vocab;
 int initialized;
 int32_t letters[26];
 char gpu[256];
#define DECLARE(name) __typeof__(&name) name;
 FUNCTIONS(DECLARE)
#undef DECLARE
};
static void ods_llama_close(struct ods_llama * b) {
 if (!b) return;
 if (b->ctx) { b->llama_synchronize(b->ctx); b->llama_free(b->ctx); }
 if (b->model) b->llama_model_free(b->model);
 if (b->initialized) b->llama_backend_free();
 // Metal libraries register Objective-C classes. Keep the DSO loaded for the
 // process lifetime; unloading/reloading it can invalidate global registrations.
 free(b);
}
static struct ods_llama * ods_llama_open(const char * library, const char * model,
                                         int threads, char * error, int error_size) {
 struct ods_llama * b = calloc(1, sizeof(*b));
 if (!b) {snprintf(error,error_size,"native allocation failed");return NULL;}
 b->library = dlopen(library, RTLD_NOW | RTLD_LOCAL);
 if (!b->library) { snprintf(error,error_size,"load llama.cpp: %s",dlerror()); goto fail; }
#define LOAD(name) b->name = (__typeof__(&name)) dlsym(b->library, #name); \
 if (!b->name) { snprintf(error,error_size,"missing native symbol: %s",#name); goto fail; }
 FUNCTIONS(LOAD)
#undef LOAD
 b->llama_backend_init(); b->initialized = 1;
 ggml_backend_dev_t gpu = b->ggml_backend_dev_by_type(GGML_BACKEND_DEVICE_TYPE_GPU);
 if (!gpu || !b->llama_supports_gpu_offload()) { snprintf(error,error_size,"Metal GPU unavailable; CPU fallback disabled"); goto fail; }
 snprintf(b->gpu,sizeof(b->gpu),"%s",b->ggml_backend_dev_description(gpu));
 struct llama_model_params mp = b->llama_model_default_params();
 ggml_backend_dev_t devices[] = {gpu, NULL};
 mp.devices = devices; mp.n_gpu_layers = -1;
 b->model = b->llama_model_load_from_file(model,mp);
 if (!b->model) { snprintf(error,error_size,"native model load failed"); goto fail; }
 b->vocab = b->llama_model_get_vocab(b->model);
 struct llama_context_params cp = b->llama_context_default_params();
 cp.n_ctx=16384; cp.n_seq_max=1; cp.n_batch=512; cp.n_ubatch=512;
 cp.n_threads=threads; cp.n_threads_batch=threads;
 cp.flash_attn_type=LLAMA_FLASH_ATTN_TYPE_DISABLED; cp.embeddings=false; cp.offload_kqv=true;
 b->ctx=b->llama_init_from_model(b->model,cp);
 if (!b->ctx || b->llama_n_ctx_seq(b->ctx)!=16384 || b->llama_n_batch(b->ctx)!=512) {snprintf(error,error_size,"native context initialization mismatch");goto fail;}
 for (int i=0;i<26;i++) {
  char letter='A'+i;
  if (b->llama_tokenize(b->vocab,&letter,1,&b->letters[i],1,false,true)!=1 || b->letters[i]!=32+i) {snprintf(error,error_size,"native candidate tokenizer mismatch");goto fail;}
 }
 return b;
fail:
 ods_llama_close(b); return NULL;
}
static int ods_llama_tokens(struct ods_llama * b,const char * text,int size,int32_t * tokens,int capacity) {
 return b->llama_tokenize(b->vocab,text,size,tokens,capacity,false,true);
}
static void ods_llama_memory(struct ods_llama * b, size_t * free_bytes, size_t * total_bytes) {
 ggml_backend_dev_t device=b->ggml_backend_dev_by_type(GGML_BACKEND_DEVICE_TYPE_GPU);
 b->ggml_backend_dev_memory(device,free_bytes,total_bytes);
}
static int ods_llama_logits(struct ods_llama * b,const int32_t * ids,int length,int count,float * out) {
 if (length<1||length>16384||count<2||count>26) return -100;
 b->llama_memory_clear(b->llama_get_memory(b->ctx),true);
 int capacity=length<512?length:512;
 struct llama_batch batch=b->llama_batch_init(capacity,0,1);
 int rc=0;
 for (int offset=0;offset<length;) {
  batch.n_tokens=length-offset<capacity?length-offset:capacity;
  for (int i=0;i<batch.n_tokens;i++) {
   batch.token[i]=ids[offset+i];batch.pos[i]=offset+i;
   batch.n_seq_id[i]=1;batch.seq_id[i][0]=0;
   batch.logits[i]=offset+i==length-1;
  }
  rc=b->llama_decode(b->ctx,batch);if (rc!=0) goto done;
  offset+=batch.n_tokens;
 }
 const float * logits=b->llama_get_logits_ith(b->ctx,batch.n_tokens-1);
 if (!logits) {rc=-101;goto done;}
 for(int i=0;i<count;i++) out[i]=logits[b->letters[i]];
done:
 if (rc!=0) {b->llama_synchronize(b->ctx);b->llama_memory_clear(b->llama_get_memory(b->ctx),true);}
 b->llama_batch_free(batch);return rc;
}
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unsafe"
)

var nativeOwned atomic.Bool

func nativeBuildError() error { return nil }

type llamaNative struct {
	ptr        *C.struct_ods_llama
	librarySHA string
}

func verifySDK(path string) error {
	dir := filepath.Dir(path)
	source, err := os.ReadFile(filepath.Join(dir, "..", "source.sha256"))
	if err != nil || strings.TrimSpace(string(source)) != SourceSHA256 {
		return fmt.Errorf("native SDK source marker missing/mismatched; use scripts/prepare-llama.sh")
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	found := false
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return fmt.Errorf("invalid SDK manifest")
		}
		name := filepath.Base(fields[1])
		if name == filepath.Base(path) {
			found = true
		}
		if err = checkHash(filepath.Join(dir, name), fields[0]); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("native library not covered by SDK manifest")
	}
	return nil
}
func openNative(opts Options) (native, error) {
	if !nativeOwned.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("one native runtime per process is supported")
	}
	success := false
	defer func() {
		if !success {
			nativeOwned.Store(false)
		}
	}()
	path, err := filepath.Abs(opts.LibraryPath)
	if err != nil {
		return nil, err
	}
	if err = verifySDK(path); err != nil {
		return nil, err
	}
	library := C.CString(path)
	model := C.CString(opts.ModelPath)
	defer C.free(unsafe.Pointer(library))
	defer C.free(unsafe.Pointer(model))
	errorBuffer := make([]byte, 2048)
	ptr := C.ods_llama_open(library, model, C.int(opts.Threads), (*C.char)(unsafe.Pointer(&errorBuffer[0])), C.int(len(errorBuffer)))
	if ptr == nil {
		return nil, fmt.Errorf("%s", strings.TrimRight(string(errorBuffer), "\x00"))
	}
	hash, err := fileHash(path)
	if err != nil {
		C.ods_llama_close(ptr)
		return nil, err
	}
	success = true
	return &llamaNative{ptr: ptr, librarySHA: hash}, nil
}
func (n *llamaNative) Tokenize(text string) ([]int32, error) {
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	needed := C.ods_llama_tokens(n.ptr, ctext, C.int(len(text)), nil, 0)
	if needed >= 0 || needed < -1048576 {
		return nil, fmt.Errorf("invalid native tokenizer size")
	}
	out := make([]int32, int(-needed))
	actual := C.ods_llama_tokens(n.ptr, ctext, C.int(len(text)), (*C.int32_t)(unsafe.Pointer(&out[0])), C.int(len(out)))
	if int(actual) != len(out) {
		return nil, fmt.Errorf("native tokenizer length mismatch")
	}
	return out, nil
}
func (n *llamaNative) Logits(ids []int32, count int) ([]float32, error) {
	if len(ids) < 1 || len(ids) > 16384 || count < 2 || count > 26 {
		return nil, fmt.Errorf("invalid native inference shape")
	}
	out := make([]float32, count)
	code := C.ods_llama_logits(n.ptr, (*C.int32_t)(unsafe.Pointer(&ids[0])), C.int(len(ids)), C.int(count), (*C.float)(unsafe.Pointer(&out[0])))
	if code != 0 {
		return nil, fmt.Errorf("llama.cpp decode failed: %d", int(code))
	}
	return out, nil
}
func (n *llamaNative) Close() error {
	C.ods_llama_close(n.ptr)
	n.ptr = nil
	nativeOwned.Store(false)
	return nil
}
func (n *llamaNative) NativeInfo() map[string]any {
	return map[string]any{"gpu": C.GoString(&n.ptr.gpu[0]), "n_ctx": 16384, "n_seq_max": 1, "n_batch": 512, "n_ubatch": 512, "gpu_layers": -1, "flash_attention": false, "source_sha256": SourceSHA256, "native_library_sha256": n.librarySHA}
}
func (n *llamaNative) NativeMetrics() string {
	var free, total C.size_t
	C.ods_llama_memory(n.ptr, &free, &total)
	return fmt.Sprintf("ods_native_device_free_memory_bytes %d\nods_native_device_total_memory_bytes %d\n", uint64(free), uint64(total))
}
