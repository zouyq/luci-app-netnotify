include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-netnotify
PKG_VERSION:=0.3.1
PKG_RELEASE:=1
PKG_LICENSE:=MIT
PKG_MAINTAINER:=zouyq <https://github.com/zouyq>

PKG_BUILD_PARALLEL:=1

include $(INCLUDE_DIR)/package.mk

GO_ARCH:=$(ARCH)
ifeq ($(ARCH),aarch64)
  GO_ARCH:=arm64
endif
ifeq ($(ARCH),x86_64)
  GO_ARCH:=amd64
endif
ifeq ($(ARCH),i386)
  GO_ARCH:=386
endif
ifeq ($(ARCH),mipsel)
  GO_ARCH:=mipsle
endif
ifeq ($(ARCH),arm)
  GO_ARCH:=arm
  GO_ARM:=7
endif

define Package/luci-app-netnotify
  SECTION:=luci
  CATEGORY:=LuCI
  SUBMENU:=3. Applications
  TITLE:=LuCI support for NetNotify
  DEPENDS:=
endef

define Package/luci-app-netnotify/description
  Event-driven LAN device online/offline push (Go daemon + LuCI JS).
endef

define Package/luci-app-netnotify/conffiles
/etc/config/netnotify
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	$(CP) ./src $(PKG_BUILD_DIR)/
	$(CP) ./root $(PKG_BUILD_DIR)/
	$(CP) ./htdocs $(PKG_BUILD_DIR)/
	[ -d ./bin ] && $(CP) ./bin $(PKG_BUILD_DIR)/ || true
endef

define Build/Compile
	mkdir -p $(PKG_BUILD_DIR)/.bin
	cd $(PKG_BUILD_DIR)/src && \
		GOCACHE=$(DL_DIR)/.gocache GOMODCACHE=$(DL_DIR)/.gomodcache \
		GOPROXY=https://proxy.golang.org,direct \
		CGO_ENABLED=0 GOOS=linux GOARCH=$(GO_ARCH) \
		$(if $(GO_ARM),GOARM=$(GO_ARM)) \
		/usr/local/go/bin/go build -trimpath -ldflags="-s -w" \
		-o $(PKG_BUILD_DIR)/.bin/netnotifyd ./cmd/netnotifyd \
		|| cp -f $(CURDIR)/bin/netnotifyd $(PKG_BUILD_DIR)/.bin/netnotifyd \
		|| cp -f $(PKG_BUILD_DIR)/bin/netnotifyd $(PKG_BUILD_DIR)/.bin/netnotifyd
	chmod +x $(PKG_BUILD_DIR)/.bin/netnotifyd
	ls -la $(PKG_BUILD_DIR)/.bin/netnotifyd
endef

define Package/luci-app-netnotify/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/.bin/netnotifyd $(1)/usr/bin/netnotifyd

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) $(PKG_BUILD_DIR)/root/etc/config/netnotify $(1)/etc/config/netnotify

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/root/etc/init.d/netnotify $(1)/etc/init.d/netnotify

	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) $(PKG_BUILD_DIR)/root/usr/share/luci/menu.d/luci-app-netnotify.json \
		$(1)/usr/share/luci/menu.d/luci-app-netnotify.json

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) $(PKG_BUILD_DIR)/root/usr/share/rpcd/acl.d/luci-app-netnotify.json \
		$(1)/usr/share/rpcd/acl.d/luci-app-netnotify.json

	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/netnotify
	$(INSTALL_DATA) $(PKG_BUILD_DIR)/htdocs/luci-static/resources/view/netnotify/*.js \
		$(1)/www/luci-static/resources/view/netnotify/

	$(INSTALL_DIR) $(1)/usr/share/netnotify
	$(INSTALL_DATA) $(PKG_BUILD_DIR)/root/usr/share/netnotify/oui_base.txt \
		$(1)/usr/share/netnotify/oui_base.txt
endef

$(eval $(call BuildPackage,luci-app-netnotify))
