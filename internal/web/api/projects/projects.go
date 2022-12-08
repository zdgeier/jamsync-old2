package api

import (
	"github.com/gin-gonic/gin"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
)

func ProjectsHandler(client jamsyncpb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.ListProjects(ctx, &jamsyncpb.ListProjectsRequest{})
		if err != nil {
			ctx.Error(err)
			return
		}
		//jsonResp, err := json.Marshal(resp)
		//if err != nil {
		//	ctx.Error(err)
		//	return
		//}
		ctx.JSON(200, resp)
	}
}
