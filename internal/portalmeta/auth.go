package portalmeta

func AuthMe() Metadata {
	return AuthRuntimeMetadata("")
}

func OAuth2Login(docsURL string) Metadata {
	return AuthRuntimeMetadata(docsURL)
}

func OAuth2Callback(docsURL string) Metadata {
	return AuthRuntimeMetadata(docsURL)
}

func OAuth2Logout(docsURL string) Metadata {
	return AuthRuntimeMetadata(docsURL)
}

func OAuth2Refresh(docsURL string) Metadata {
	return AuthRuntimeMetadata(docsURL)
}
