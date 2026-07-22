package setting

// Waffo Pancake hosted checkout configuration. Gateway is enabled once
// MerchantID + PrivateKey + ProductID are populated (no separate Enabled
// flag, matching Stripe / Creem). StoreID + ProductID are operator-bound
// via SaveWaffoPancakeConfig.
var (
	WaffoPancakeMerchantID  string
	WaffoPancakePrivateKey  string
	WaffoPancakeReturnURL   string
	WaffoPancakeEnvironment string
	WaffoPancakeUnitPrice   float64 = 1.0
	WaffoPancakeMinTopUp    int     = 1
	WaffoPancakeStoreID     string
	WaffoPancakeProductID   string
)

const (
	WaffoPancakeEnvironmentTest = "test"
	WaffoPancakeEnvironmentProd = "prod"
)

func IsValidWaffoPancakeEnvironment(environment string) bool {
	return environment == WaffoPancakeEnvironmentTest || environment == WaffoPancakeEnvironmentProd
}
