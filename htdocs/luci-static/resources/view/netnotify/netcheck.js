'use strict';
'require form';
'require uci';
'require view';

return view.extend({
	load: function () {
		return Promise.all([
			uci.load('netnotify')
		]);
	},

	render: function () {
		var m, s, o;

		m = new form.Map('netnotify', _('网络检测'),
			_('检测公网连通性：先探测 generate_204 网址，失败后再探测 IP；全部失败则重启 WAN。恢复后可推送状态消息。'));

		s = m.section(form.TypedSection, 'netnotify', _('检测开关'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Flag, 'netcheck_enable', _('启用网络检测'));
		o.default = '0';
		o.rmempty = false;
		o.description = _('开启后由 netnotifyd 按间隔检测，无需再单独跑 network_check.sh');

		o = s.option(form.Flag, 'netcheck_push_on_recover', _('恢复后推送通知'));
		o.default = '1';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');
		o.description = _('WAN 重启后网络恢复成功时，通过当前推送渠道发送 WAN IP / 负载 / 运行时长等信息');

		s = m.section(form.TypedSection, 'netnotify', _('探测目标'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.DynamicList, 'netcheck_hosts', _('检测网址（主机名）'));
		o.placeholder = 'connect.rom.miui.com';
		o.rmempty = true;
		o.depends('netcheck_enable', '1');
		o.description = _('访问 http://主机/generate_204，期望返回 204。可增减，按顺序探测，任一成功即判定在线');

		o = s.option(form.DynamicList, 'netcheck_ips', _('备用检测 IP'));
		o.placeholder = '223.5.5.5';
		o.datatype = 'ipaddr';
		o.rmempty = true;
		o.depends('netcheck_enable', '1');
		o.description = _('网址全部失败后，再 ping/探测这些 IP（默认阿里 DNS / DNSPod）');

		s = m.section(form.TypedSection, 'netnotify', _('时间与动作'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Value, 'netcheck_interval_sec', _('检测间隔（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '300';
		o.default = '300';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');

		o = s.option(form.Value, 'netcheck_timeout_sec', _('单次超时（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '2';
		o.default = '2';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');

		o = s.option(form.Value, 'netcheck_retry', _('每个网址重试次数'));
		o.datatype = 'uinteger';
		o.placeholder = '2';
		o.default = '2';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');

		o = s.option(form.Value, 'netcheck_retry_interval_sec', _('重试间隔（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '1';
		o.default = '1';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');

		o = s.option(form.Value, 'netcheck_wan_iface', _('WAN 接口名'));
		o.placeholder = 'wan';
		o.default = 'wan';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');
		o.description = _('失败时执行 /sbin/ifup <接口>，一般为 wan 或 pppoe-wan 对应的逻辑口');

		o = s.option(form.Value, 'netcheck_startup_wait_sec', _('启动等待网络（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '120';
		o.default = '120';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');
		o.description = _('守护进程启动后先等待网络就绪，再开始周期检测');

		o = s.option(form.Value, 'netcheck_cooldown_sec', _('重启冷却时间（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '600';
		o.default = '600';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');
		o.description = _('两次 ifup 最短间隔，避免反复重启 WAN');

		o = s.option(form.Value, 'netcheck_recover_wait_sec', _('重启后等待恢复（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '120';
		o.default = '120';
		o.rmempty = false;
		o.depends('netcheck_enable', '1');

		return m.render();
	}
});
