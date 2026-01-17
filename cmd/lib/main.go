package main

/*
#include <stdlib.h>

// Define callback function types with void* user_data for context/reference passing
typedef void (*ProgressCallback)(char* phase, int done, int total, void* user_data);
typedef void (*ErrorCallback)(char* stage, char* error, void* user_data);
typedef void (*TranslatedCallback)(char* original, char* translated, void* user_data);
typedef void (*CompleteCallback)(char* error, void* user_data);

// Helper functions to call the function pointers from Go
static void call_progress(ProgressCallback cb, char* phase, int done, int total, void* user_data) {
    if (cb) cb(phase, done, total, user_data);
}

static void call_error(ErrorCallback cb, char* stage, char* error, void* user_data) {
    if (cb) cb(stage, error, user_data);
}

static void call_translated(TranslatedCallback cb, char* original, char* translated, void* user_data) {
    if (cb) cb(original, translated, user_data);
}

static void call_complete(CompleteCallback cb, char* error, void* user_data) {
    if (cb) cb(error, user_data);
}
*/
import "C"
import (
	"context"
	"exceltranslator/pkg/config"
	"exceltranslator/pkg/runner"
	"sync"
	"unsafe"

	"github.com/pelletier/go-toml/v2"
)

var taskMap sync.Map // map[int64]context.CancelFunc

//export Translate
func Translate(
	taskID C.longlong,
	inputPath *C.char,
	outputPath *C.char,
	configToml *C.char,
	progressCB C.ProgressCallback,
	errorCB C.ErrorCallback,
	translatedCB C.TranslatedCallback,
	completeCB C.CompleteCallback,
	userData unsafe.Pointer,
) *C.char {
	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	id := int64(taskID)
	taskMap.Store(id, cancel)
	defer func() {
		taskMap.Delete(id)
		cancel()
	}()

	// Convert C strings to Go strings
	goInput := C.GoString(inputPath)
	goOutput := C.GoString(outputPath)
	goConfigToml := C.GoString(configToml)

	// Parse config
	var cfg config.AppConfig
	if err := toml.Unmarshal([]byte(goConfigToml), &cfg); err != nil {
		return C.CString("failed to parse config toml: " + err.Error())
	}

	// Map Go callbacks to C callbacks
	cb := runner.TranslationCallbacks{
		OnTranslated: func(original, translated string) {
			if translatedCB != nil {
				cOriginal := C.CString(original)
				cTranslated := C.CString(translated)
				defer C.free(unsafe.Pointer(cOriginal))
				defer C.free(unsafe.Pointer(cTranslated))
				C.call_translated(translatedCB, cOriginal, cTranslated, userData)
			}
		},
		OnProgress: func(phase string, done, total int) {
			if progressCB != nil {
				cPhase := C.CString(phase)
				defer C.free(unsafe.Pointer(cPhase))
				C.call_progress(progressCB, cPhase, C.int(done), C.int(total), userData)
			}
		},
		OnError: func(stage string, err error) {
			if errorCB != nil {
				cStage := C.CString(stage)
				cErr := C.CString(err.Error())
				defer C.free(unsafe.Pointer(cStage))
				defer C.free(unsafe.Pointer(cErr))
				C.call_error(errorCB, cStage, cErr, userData)
			}
		},
		OnComplete: func(err error) {
			if completeCB != nil {
				var cErr *C.char
				if err != nil {
					cErr = C.CString(err.Error())
					defer C.free(unsafe.Pointer(cErr))
				}
				C.call_complete(completeCB, cErr, userData)
			}
		},
	}

	err := runner.RunTranslationWithConfig(ctx, goInput, goOutput, &cfg, cb)

	// OnComplete will be called by the runner, but we also return the error
	// for synchronous error handling if needed
	if err != nil {
		return C.CString(err.Error())
	}

	return nil // Success
}

//export CancelTranslate
func CancelTranslate(taskID C.longlong) {
	if val, ok := taskMap.Load(int64(taskID)); ok {
		if cancel, ok := val.(context.CancelFunc); ok {
			cancel()
		}
	}
}

func main() {}
