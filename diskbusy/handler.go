package diskbusy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"

	"github.com/docker/go-units"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type DiskBusyHandler struct {
	fileDir       string
	runningWorker map[string]context.CancelFunc
}

func choose(items []string) string {
	return items[rand.Int31n(int32(len(items)))]
}

func (h *DiskBusyHandler) run(ctx context.Context, limiter *rate.Limiter) error {
	items, err := filepath.Glob(h.fileDir)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("glob %s expands to empty set", h.fileDir)
	}
	err = RunBusyWork(ctx, func() string {
		return choose(items)
	}, limiter)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf("WARN: file not found, the file dir might be modified. (err = %s)", err)
		return h.run(ctx, limiter)
	}

	return err
}

func (h *DiskBusyHandler) fork(ctx context.Context, parN int, ratelimit int64) (string, error) {
	id := uuid.New().String()
	cctx, cancel := context.WithCancel(ctx)
	for i := 0; i < parN; i++ {
		limiter := rate.NewLimiter(rate.Limit(ratelimit), int(ratelimit))
		go func() {
			if err := h.run(cctx, limiter); err != nil {
				log.Printf("WARN: error during running busy work (err = %s, id = %s)", err, id)
			}
		}()
	}
	h.runningWorker[id] = cancel
	return id, nil
}

type AddBusyRequest struct {
	ParN      int    `json:"n"`
	RateLimit string `json:"rate_limit"`
}

type AddBusyResponse struct {
	ID string `json:"id"`
}

func (h *DiskBusyHandler) handleAdd(ctx *gin.Context) {
	req := AddBusyRequest{}
	defer func() {
		if ctx.Errors.Last() != nil {
			ctx.JSON(http.StatusInternalServerError, ctx.Errors)
			log.Printf("error during handling request (req = %#v)", req)
		}
	}()
	if err := ctx.Bind(&req); err != nil {
		ctx.Error(err)
		return
	}
	rate, err := units.FromHumanSize(req.RateLimit)
	if err != nil {
		ctx.Error(err)
		return
	}
	id, err := h.fork(context.TODO(), req.ParN, rate)
	if err != nil {
		ctx.Error(err)
		return
	}
	log.Printf("Successfully added workload (ratelimit = %s/s, id = %s, threads = %d)", req.RateLimit, id, req.ParN)
	ctx.JSON(201, AddBusyResponse{
		ID: id,
	})
}

func (h *DiskBusyHandler) handleDelete(ctx *gin.Context) {
	id := ctx.Param("id")
	cancel, ok := h.runningWorker[id]
	if !ok {
		ctx.String(http.StatusNotFound, "id %q not found", id)
		return
	}
	cancel()
	log.Printf("Deleted workload (id = %s)", id)
	ctx.Status(http.StatusOK)
}

func (h *DiskBusyHandler) Register(r gin.IRouter) {
	r.POST("/busy", h.handleAdd)
	r.DELETE("/busy/:id", h.handleDelete)
}

func NewHandler(filepath string) *DiskBusyHandler {
	return &DiskBusyHandler{
		fileDir:       filepath,
		runningWorker: make(map[string]context.CancelFunc),
	}
}
