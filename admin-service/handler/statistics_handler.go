package handler

import (
	"fmt"
	"strconv"
	"time"

	"social-network-go/admin-service/model"

	"github.com/gin-gonic/gin"
)

func (h *AdminHandler) GetUsersStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	stats, err := h.svc.GetUsersStatistics(ctx)
	if err != nil {
		stats = &model.UserStatisticsResponse{}
	}

	stats.OnlineUsersNow = h.svc.GetOnlineUsersCount()
	stats.OnlineStatistics = h.svc.GetOnlineStatisticsLogs("")

	now := time.Now()
	_, isoWeek := now.ISOWeek()
	year := now.Year()
	month := int(now.Month())
	stats.ThisWeekStatistics = h.svc.QueryWeekUserStats(ctx, isoWeek, year)
	stats.ThisMonthStatistics = h.svc.QueryMonthUserStats(ctx, month, year)
	stats.ThisYearStatistics = h.svc.QueryYearUserStats(ctx, year)

	sendSuccess(c, stats)
}

func (h *AdminHandler) GetUsersWeekStatistics(c *gin.Context) {
	weekStr := c.DefaultQuery("week", "")
	var week, year int
	if weekStr != "" {
		fmt.Sscanf(weekStr, "%d-W%d", &year, &week)
	}
	if year == 0 || week == 0 {
		_, week = time.Now().ISOWeek()
		year = time.Now().Year()
	}
	sendSuccess(c, h.svc.QueryWeekUserStats(c.Request.Context(), week, year))
}

func (h *AdminHandler) GetUsersMonthStatistics(c *gin.Context) {
	monthStr := c.DefaultQuery("month", "")
	var year, month int
	if monthStr != "" {
		fmt.Sscanf(monthStr, "%d-%d", &year, &month)
	}
	if year == 0 || month == 0 {
		year = time.Now().Year()
		month = int(time.Now().Month())
	}
	sendSuccess(c, h.svc.QueryMonthUserStats(c.Request.Context(), month, year))
}

func (h *AdminHandler) GetUsersYearStatistics(c *gin.Context) {
	year := time.Now().Year()
	if y, err := strconv.Atoi(c.DefaultQuery("year", "")); err == nil && y > 0 {
		year = y
	}
	sendSuccess(c, h.svc.QueryYearUserStats(c.Request.Context(), year))
}

func (h *AdminHandler) GetUsersOnlineStatistics(c *gin.Context) {
	dateStr := c.Query("date")
	sendSuccess(c, h.svc.GetOnlineStatisticsLogs(dateStr))
}

func (h *AdminHandler) GetPostsStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	stats, err := h.svc.GetPostsStatistics(ctx)
	if err != nil {
		stats = &model.PostStatisticsResponse{}
	}

	pNow := time.Now()
	_, pWeek := pNow.ISOWeek()
	pYear := pNow.Year()
	pMonth := int(pNow.Month())
	stats.ThisWeekStatistics = h.svc.QueryWeekPostStats(ctx, pWeek, pYear)
	stats.ThisMonthStatistics = h.svc.QueryMonthPostStats(ctx, pMonth, pYear)
	stats.ThisYearStatistics = h.svc.QueryYearPostStats(ctx, pYear)

	sendSuccess(c, stats)
}

func (h *AdminHandler) GetPostsWeekStatistics(c *gin.Context) {
	weekStr := c.DefaultQuery("week", "")
	var week, year int
	if weekStr != "" {
		fmt.Sscanf(weekStr, "%d-W%d", &year, &week)
	}
	if year == 0 || week == 0 {
		_, week = time.Now().ISOWeek()
		year = time.Now().Year()
	}
	sendSuccess(c, h.svc.QueryWeekPostStats(c.Request.Context(), week, year))
}

func (h *AdminHandler) GetPostsMonthStatistics(c *gin.Context) {
	monthStr := c.DefaultQuery("month", "")
	var year, month int
	if monthStr != "" {
		fmt.Sscanf(monthStr, "%d-%d", &year, &month)
	}
	if year == 0 || month == 0 {
		year = time.Now().Year()
		month = int(time.Now().Month())
	}
	sendSuccess(c, h.svc.QueryMonthPostStats(c.Request.Context(), month, year))
}

func (h *AdminHandler) GetPostsYearStatistics(c *gin.Context) {
	year := time.Now().Year()
	if y, err := strconv.Atoi(c.DefaultQuery("year", "")); err == nil && y > 0 {
		year = y
	}
	sendSuccess(c, h.svc.QueryYearPostStats(c.Request.Context(), year))
}
