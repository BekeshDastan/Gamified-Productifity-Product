package main

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/push"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
)

func main() {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := platform.NewLogger(os.Stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("invalid database configuration")
		os.Exit(1)
	}
	defer pool.Close()
	authConfig, err := auth.LoadSessionConfig(cfg.Origin, cfg.Environment, os.LookupEnv)
	if err != nil {
		logger.Error("invalid auth configuration")
		os.Exit(1)
	}
	sessions, err := auth.NewSessions(ctx, auth.NewService(pool, auth.NewPasswordHasher(), platform.RealClock{}), authConfig)
	if err != nil {
		logger.Error("invalid auth configuration")
		os.Exit(1)
	}
	smtpAdapter, err := mail.NewSMTP(cfg.Environment, net.JoinHostPort(os.Getenv("SMTP_HOST"), os.Getenv("SMTP_PORT")), os.Getenv("MAIL_FROM"), os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD"))
	if err != nil {
		logger.Warn("mail_adapter_unavailable")
	}
	var mailer mail.Mailer = mail.Unavailable{}
	if smtpAdapter != nil {
		mailer = smtpAdapter
	}
	logger.Info("worker starting")
	clock := platform.RealClock{}
	rewardService := progression.NewService(pool, clock)
	characterService := character.NewService(pool, clock, rewardService)
	rewardService.WithProjectionObserver(characterService.EvaluateInTx)
	go calendar.NewService(pool, clock, quests.NewService(pool, clock).WithObserver(goals.NewService(pool, clock).QuestChanged).WithProgression(rewardService)).Run(ctx, logger)
	handlers := sessions.MailHandlers(mailer)
	analyticsService := analytics.NewService(pool, clock).WithRecords(characterService)
	if err := analyticsService.StartReportSchedule(ctx); err != nil {
		logger.Error("report_schedule_failed")
		os.Exit(1)
	}
	notificationService := notifications.NewService(pool, clock).WithHabits(habits.NewService(pool, clock).WithProgression(rewardService))
	characterService.WithAchievementObserver(notificationService.AchievementChanged)
	notificationService.WithFocusSettlement(focus.NewService(pool, clock, rewardService).WithResultObserver(notificationService.FocusChanged).SettleDueInTx)
	analyticsService.WithReportObserver(notificationService.ReportCreated)

	publicKey, privateKey := os.Getenv("PUSH_VAPID_PUBLIC_KEY"), os.Getenv("PUSH_VAPID_PRIVATE_KEY")
	if publicKey != "" && privateKey != "" {
		raw, e := base64.RawURLEncoding.DecodeString(privateKey)
		if e != nil {
			logger.Error("invalid push configuration")
			os.Exit(1)
		}
		key, e := ecdh.P256().NewPrivateKey(raw)
		if e != nil || base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()) != publicKey {
			logger.Error("invalid push configuration")
			os.Exit(1)
		}
		notificationService.ConfigurePush(publicKey, push.Adapter{PublicKey: publicKey, PrivateKey: privateKey, Subject: push.Contact(cfg.Origin, os.Getenv("MAIL_FROM"))})
	}

	if err := notificationService.ConfigureDelivery(cfg.Origin, authConfig.SigningKey, mailer); err != nil {
		logger.Error("notification_configuration_failed")
		os.Exit(1)
	}
	if err := notificationService.StartReminderSchedule(ctx); err != nil {
		logger.Error("reminder_schedule_failed")
		os.Exit(1)
	}
	for kind, handler := range notificationService.Handlers() {
		handlers[kind] = handler
	}
	for kind, handler := range analyticsService.Handlers() {
		handlers[kind] = handler
	}
	challengeService := challenges.NewService(pool, clock).WithProgression(rewardService)
	for kind, handler := range seasons.NewService(pool, clock).WithChallenges(challengeService).Handlers() {
		handlers[kind] = handler
	}
	for kind, handler := range challengeService.Handlers() {
		handlers[kind] = handler
	}
	for kind, handler := range characterService.Handlers() {
		handlers[kind] = handler
	}
	for kind, handler := range rewardService.Handlers() {
		handlers[kind] = handler
	}
	jobs.Run(ctx, jobs.Store{Pool: pool, Clock: platform.RealClock{}}, handlers, logger)
	logger.Info("worker stopped")
}
