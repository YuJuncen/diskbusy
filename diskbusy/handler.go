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
	runningWorker map[string][]context.CancelFunc
}

func choose(items []string) string {
	return items[rand.Int31n(int32(len(items)))]
}

const levelCount = 7

func (h *DiskBusyHandler) Run(ctx context.Context, limiter *rate.Limiter) error {
	items, err := filepath.Glob(h.fileDir)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("glob %s expands to empty set", h.fileDir)
	}
	err = RunBusyWork(ctx, func() string {
		return choose(items)
	}, levelCount, limiter)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf("WARN: file not found, the file dir might be modified. (err = %s)", err)
		return h.Run(ctx, limiter)
	}

	return err
}

func (h *DiskBusyHandler) Fork(ctx context.Context, parN int, ratelimit int64) (string, error) {
	id := uuid.New().String()
	for i := 0; i < parN; i++ {
		limiter := rate.NewLimiter(rate.Limit(ratelimit), int(ratelimit))
		cctx, cancel := context.WithCancel(ctx)
		go func() {
			if err := h.Run(cctx, limiter); err != nil {
				log.Printf("WARN: error during running busy work (err = %s, id = %s)", err, id)
			}
		}()
		h.runningWorker[id] = append(h.runningWorker[id], cancel)
	}
	return id, nil
}

type AddBusyRequest struct {
	ParN      int    `json:"n"`
	RateLimit string `json:"rate_limit"`
}

type AddBusyResponse struct {
	ID string `json:"id"`
}

func (h *DiskBusyHandler) HandleAdd(ctx *gin.Context) {
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
	id, err := h.Fork(context.TODO(), req.ParN, rate)
	if err != nil {
		ctx.Error(err)
		return
	}
	log.Printf("Successfully added workload (ratelimit = %s/s, id = %s, threads = %d)", req.RateLimit, id, req.ParN)
	ctx.JSON(201, AddBusyResponse{
		ID: id,
	})
}

func (h *DiskBusyHandler) HandleDelete(ctx *gin.Context) {
	id := ctx.Param("id")
	cancels, ok := h.runningWorker[id]
	if !ok {
		ctx.String(http.StatusNotFound, "id %q not found", id)
		return
	}
	for _, cancel := range cancels {
		cancel()
	}
	log.Printf("Deleted workload (id = %s)", id)
	ctx.Status(http.StatusOK)
}

func (h *DiskBusyHandler) Register(r gin.IRouter) {
	r.POST("/busy", h.HandleAdd)
	r.DELETE("/busy/:id", h.HandleDelete)
}

func NewHandler(filepath string) *DiskBusyHandler {
	return &DiskBusyHandler{
		fileDir:       filepath,
		runningWorker: make(map[string][]context.CancelFunc),
	}
}
