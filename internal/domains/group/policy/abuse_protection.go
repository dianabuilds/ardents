package policy

import (
	"os"
	"strconv"
	"strings"
	"time"

	"aim-chat/go-backend/internal/platform/ratelimiter"
)

const (
	groupMaxMembersEnv               = "AIM_GROUP_MAX_MEMBERS"
	groupMaxPendingInvitesEnv        = "AIM_GROUP_MAX_PENDING_INVITES"
	groupInviteRateLimitRPSEnv       = "AIM_GROUP_INVITE_RATE_LIMIT_RPS"
	groupInviteRateLimitBurstEnv     = "AIM_GROUP_INVITE_RATE_LIMIT_BURST"
	groupSendRateLimitRPSEnv         = "AIM_GROUP_SEND_RATE_LIMIT_RPS"
	groupSendRateLimitBurstEnv       = "AIM_GROUP_SEND_RATE_LIMIT_BURST"
	groupMembershipRateLimitRPSEnv   = "AIM_GROUP_MEMBERSHIP_RATE_LIMIT_RPS"
	groupMembershipRateLimitBurstEnv = "AIM_GROUP_MEMBERSHIP_RATE_LIMIT_BURST"
)

type abuseProtectionConfig struct {
	MaxMembers               int
	MaxPendingInvites        int
	InviteRateLimitRPS       float64
	InviteRateLimitBurst     int
	SendRateLimitRPS         float64
	SendRateLimitBurst       int
	MembershipRateLimitRPS   float64
	MembershipRateLimitBurst int
}

func loadAbuseProtectionConfigFromEnv() abuseProtectionConfig {
	cfg := abuseProtectionConfig{
		MaxMembers:               128,
		MaxPendingInvites:        64,
		InviteRateLimitRPS:       100,
		InviteRateLimitBurst:     200,
		SendRateLimitRPS:         200,
		SendRateLimitBurst:       400,
		MembershipRateLimitRPS:   100,
		MembershipRateLimitBurst: 200,
	}
	cfg.MaxMembers = readPositiveIntEnv(groupMaxMembersEnv, cfg.MaxMembers)
	cfg.MaxPendingInvites = readPositiveIntEnv(groupMaxPendingInvitesEnv, cfg.MaxPendingInvites)
	cfg.InviteRateLimitRPS = readPositiveFloatEnv(groupInviteRateLimitRPSEnv, cfg.InviteRateLimitRPS)
	cfg.InviteRateLimitBurst = readPositiveIntEnv(groupInviteRateLimitBurstEnv, cfg.InviteRateLimitBurst)
	cfg.SendRateLimitRPS = readPositiveFloatEnv(groupSendRateLimitRPSEnv, cfg.SendRateLimitRPS)
	cfg.SendRateLimitBurst = readPositiveIntEnv(groupSendRateLimitBurstEnv, cfg.SendRateLimitBurst)
	cfg.MembershipRateLimitRPS = readPositiveFloatEnv(groupMembershipRateLimitRPSEnv, cfg.MembershipRateLimitRPS)
	cfg.MembershipRateLimitBurst = readPositiveIntEnv(groupMembershipRateLimitBurstEnv, cfg.MembershipRateLimitBurst)
	if cfg.MaxPendingInvites > cfg.MaxMembers {
		cfg.MaxPendingInvites = cfg.MaxMembers
	}
	return cfg
}

func readPositiveIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func readPositiveFloatEnv(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

type operationRateLimiter struct {
	limiter *ratelimiter.MapLimiter
}

func newOperationRateLimiter(rps float64, burst int) *operationRateLimiter {
	limiter := ratelimiter.New(rps, burst, 10*time.Minute)
	if limiter == nil {
		return nil
	}
	return &operationRateLimiter{
		limiter: limiter,
	}
}

func (l *operationRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	return l.limiter.Allow(key, now)
}

type AbuseProtection struct {
	cfg               abuseProtectionConfig
	inviteLimiter     *operationRateLimiter
	sendLimiter       *operationRateLimiter
	membershipLimiter *operationRateLimiter
}

func NewAbuseProtectionFromEnv() *AbuseProtection {
	cfg := loadAbuseProtectionConfigFromEnv()
	return &AbuseProtection{
		cfg:               cfg,
		inviteLimiter:     newOperationRateLimiter(cfg.InviteRateLimitRPS, cfg.InviteRateLimitBurst),
		sendLimiter:       newOperationRateLimiter(cfg.SendRateLimitRPS, cfg.SendRateLimitBurst),
		membershipLimiter: newOperationRateLimiter(cfg.MembershipRateLimitRPS, cfg.MembershipRateLimitBurst),
	}
}

func (p *AbuseProtection) AllowInvite(actorID string, now time.Time) bool {
	return allowByLimiter(p, p.inviteLimiter, actorID, now)
}

func (p *AbuseProtection) AllowSend(actorID string, now time.Time) bool {
	return allowByLimiter(p, p.sendLimiter, actorID, now)
}

func (p *AbuseProtection) AllowMembership(actorID string, now time.Time) bool {
	return allowByLimiter(p, p.membershipLimiter, actorID, now)
}

func allowByLimiter(p *AbuseProtection, limiter *operationRateLimiter, actorID string, now time.Time) bool {
	if p == nil {
		return true
	}
	return limiter.Allow(actorID, now)
}

func (p *AbuseProtection) EnforceInviteQuotas(state GroupState) error {
	if p == nil {
		return nil
	}
	memberCount := 0
	pendingInvites := 0
	for _, member := range state.Members {
		if member.Status == GroupMemberStatusActive || member.Status == GroupMemberStatusInvited {
			memberCount++
		}
		if member.Status == GroupMemberStatusInvited {
			pendingInvites++
		}
	}
	if memberCount >= p.cfg.MaxMembers {
		return ErrGroupMemberLimitExceeded
	}
	if pendingInvites >= p.cfg.MaxPendingInvites {
		return ErrGroupPendingInvitesLimitExceeded
	}
	return nil
}
