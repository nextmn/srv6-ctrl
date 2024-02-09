// Copyright 2023 Louis Royer and the NextMN-SRv6-ctrl contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package ctrl

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
	"github.com/nextmn/srv6-ctrl/json_api"
)

type HttpServerEntity struct {
	srv     *http.Server
	routers *RouterRegistry
}

type RouterRegistry struct {
	sync.RWMutex
	routers json_api.RouterMap
}

func NewHttpServerEntity(addr string, port string) *HttpServerEntity {
	rr := RouterRegistry{
		routers: make(json_api.RouterMap),
	}
	r := gin.Default()
	r.GET("/status", rr.Status)
	r.GET("/routers", rr.GetRouters)
	r.GET("/routers/:uuid", rr.GetRouter)
	r.DELETE("/routers/:uuid", rr.DeleteRouter)
	r.POST("/routers", rr.PostRouter)
	httpAddr := fmt.Sprintf("[%s]:%s", addr, port)
	log.Printf("HTTP Server will be listenning on %s\n", httpAddr)
	e := HttpServerEntity{
		routers: &rr,
		srv: &http.Server{
			Addr:    httpAddr,
			Handler: r,
		},
	}
	return &e
}

func (e *HttpServerEntity) Start() {
	go func() {
		if err := e.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("listen: %s\n", err)
		}
	}()
}

func (e *HttpServerEntity) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := e.srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP Server Shutdown: %s\n", err)
	}
}

// get status of the controller
func (l *RouterRegistry) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.JSON(http.StatusOK, gin.H{"ready": true})
}

// get a router infos
func (r *RouterRegistry) GetRouter(c *gin.Context) {
	id := c.Param("uuid")
	idUuid, err := uuid.FromString(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "bad uuid", "error": fmt.Sprintf("%v", err)})
		return
	}
	c.Header("Cache-Control", "no-cache")
	r.RLock()
	defer r.RUnlock()
	if val, ok := r.routers[idUuid]; ok {
		c.JSON(http.StatusOK, val)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"message": "router not found"})
}

// post a router infos
func (r *RouterRegistry) PostRouter(c *gin.Context) {
	var router json_api.Router
	if err := c.BindJSON(&router); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "could not deserialize", "error": fmt.Sprintf("%v", err)})
		return
	}
	c.Header("Cache-Control", "no-cache")
	r.Lock()
	defer r.Unlock()
	for k, v := range r.routers {
		if router.Locator.Overlaps(v.Locator) {
			c.JSON(http.StatusConflict, gin.H{"message": "This locator overlaps with locator of router " + k.String()})
			return
		}
	}

	id, err := uuid.NewV4()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to generate UUID"})
	}
	for {
		if _, exists := r.routers[id]; !exists {
			break
		} else {
			id, err = uuid.NewV4()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to generate UUID"})
			}
		}
	}
	r.routers[id] = router
	c.Header("Location", fmt.Sprintf("/routers/%s", id))
	c.JSON(http.StatusCreated, r.routers[id])
}

func (r *RouterRegistry) DeleteRouter(c *gin.Context) {
	id := c.Param("uuid")
	idUuid, err := uuid.FromString(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "bad uuid", "error": fmt.Sprintf("%v", err)})
		return
	}
	c.Header("Cache-Control", "no-cache")
	r.Lock()
	defer r.Unlock()
	if _, exists := r.routers[idUuid]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"message": "router not found"})
		return
	}

	delete(r.routers, idUuid)
	c.Status(http.StatusNoContent) // successful deletion
}

func (r *RouterRegistry) GetRouters(c *gin.Context) {
	c.JSON(http.StatusOK, r.routers)
}
