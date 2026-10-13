package shell

func personalSettingsItems(administrator bool) []Item {
	securityLabel := "Security"
	if administrator {
		securityLabel = "Security & sessions"
	}
	items := []Item{
		{ID: "profile", Label: "Profile", Href: "/admin/profile", Icon: "user"},
		{ID: "appearance", Label: "Appearance", Href: "/admin/appearance", Icon: "palette"},
		{ID: "security", Label: securityLabel, Href: "/admin/security", Icon: "activity"},
	}
	if administrator {
		items = append(items, Item{ID: "api-tokens", Label: "API tokens", Href: "/admin/api-tokens", Icon: "data"})
	}
	return append(items, Item{ID: "archived-chats", Label: "Archived chats", Href: "/admin/archived-chats", Icon: "archive"})
}
