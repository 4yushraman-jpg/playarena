-- =============================================================================
-- Migration  : 000026_webhook_notifications (DOWN)
-- =============================================================================

DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
