package transport

const (
	MethodIdentityGet        = "identity.get"
	MethodIdentitySelfCard   = "identity.self_contact_card"
	MethodIdentityLogin      = "identity.login"
	MethodIdentityCreate     = "identity.create"
	MethodIdentityExportSeed = "identity.export_seed"
	MethodIdentityImportSeed = "identity.import_seed"
	MethodIdentityMnemonic   = "identity.validate_mnemonic"
	MethodIdentityChangePwd  = "identity.change_password"
	MethodBackupExport       = "backup.export"
	MethodBackupRestore      = "backup.restore"
	MethodDataWipe           = "data.wipe"
	MethodAccountList        = "account.list"
	MethodAccountCurrent     = "account.current"
	MethodAccountSwitch      = "account.switch"
)
