const inviteCardHtml = `
  <div class="border border-slate-200 rounded-xl overflow-hidden bg-white">
    <div class="h-2 bg-[#009BF5] w-full"></div>
    <div class="p-8">
      <div class="text-2xl font-bold text-slate-900 mb-2">诚邀入驻悦享e食</div>
      <div class="text-slate-500 mb-8">开启您的线上餐厅新增长</div>
      <div class="space-y-4 text-slate-700 text-sm leading-relaxed mb-8">
        <p>尊敬的商户伙伴：</p>
        <p>我们诚挚地邀请您加入悦享e食生态。在这里，您将获得全城数百万活跃用户的流量支持，以及专业的数字化经营工具。</p>
        <ul class="list-disc pl-5 space-y-1 text-slate-600">
          <li>首月免佣金福利</li>
          <li>一对一运营指导</li>
          <li>专属骑手运力保障</li>
        </ul>
      </div>
      <div class="text-center">
        <a href="#" class="inline-block bg-[#009BF5] text-white px-8 py-3 rounded-full font-medium">点击立即入驻</a>
      </div>
    </div>
    <div class="bg-slate-50 p-4 text-center text-xs text-slate-400 border-t border-slate-100">
      此邮件由悦享邮局自动生成
    </div>
  </div>
`;

export function createSeedState() {
  return {
    auth: {
      isAuthenticated: false,
    },
    profile: {
      id: "user-10001",
      displayName: "MyName",
      rolePrefix: "user-",
      emailPrefix: "",
      email: "",
      avatarInitial: "M",
      unifiedAccountPhone: "138****8888",
      mailboxProvisioned: false,
      mailboxDomain: "yuexiang.com",
    },
    health: {
      status: "healthy",
      label: "服务连接正常",
      source: "mock",
    },
    settings: {
      defaultSenderName: "悦享用户_MyName",
      signature: "--\n此致\n悦享e食 合作伙伴",
      autoReplyEnabled: false,
      autoReplyMessage: "您好，我暂时无法及时回复，稍后会第一时间处理您的来信。",
    },
    contacts: [
      {
        id: "contact-1",
        name: "李设计",
        email: "li.design@yuexiang.com",
        avatar: "L",
        role: "内部员工",
        organization: "品牌设计中心",
        lastContactedAt: "昨天",
        note: "负责品牌主视觉与活动物料",
      },
      {
        id: "contact-2",
        name: "张店长",
        email: "zhang.boss@external.com",
        avatar: "Z",
        role: "外部商户",
        organization: "张记小馆",
        lastContactedAt: "5月1日",
        note: "重点连锁合作商户",
      },
      {
        id: "contact-3",
        name: "骑手运营组",
        email: "rider.ops@yuexiang.com",
        avatar: "R",
        role: "平台官方",
        organization: "运力运营部",
        lastContactedAt: "4月29日",
        note: "骑手招募、培训和激励政策",
      },
      {
        id: "contact-4",
        name: "周体验官",
        email: "beta.user@example.com",
        avatar: "Z",
        role: "核心用户",
        organization: "悦享内测群",
        lastContactedAt: "4月26日",
        note: "参与产品可用性内测",
      },
      {
        id: "contact-5",
        name: "王商务",
        email: "wang.bd@yuexiang.com",
        avatar: "W",
        role: "内部员工",
        organization: "商务拓展部",
        lastContactedAt: "4月21日",
        note: "平台招商和KA商户合作",
      },
    ],
    templates: [
      {
        id: "invite-merchant",
        role: "merchant",
        subject: "诚邀入驻悦享e食",
        html: inviteCardHtml,
      },
      {
        id: "invite-rider",
        role: "rider",
        subject: "诚邀加入悦享骑手团队",
        html: inviteCardHtml.replace("诚邀入驻悦享e食", "诚邀加入悦享骑手团队").replace("商户伙伴", "骑手伙伴"),
      },
      {
        id: "invite-user",
        role: "user",
        subject: "悦享e食核心用户内测邀请",
        html: inviteCardHtml.replace("诚邀入驻悦享e食", "悦享e食核心用户内测邀请").replace("商户伙伴", "体验官"),
      },
    ],
    messages: [
      {
        id: "mail-1",
        folder: "inbox",
        previousFolder: "inbox",
        sender: "悦享e食平台组",
        senderEmail: "system@yuexiang.com",
        recipients: ["myname@yuexiang.com"],
        avatar: "Y",
        role: "平台官方",
        subject: "您的商户入驻申请已通过",
        snippet: "尊敬的用户，您提交的商户入驻资料已审核通过，请登录控制台...",
        time: "10:42",
        dateTimeLabel: "2026年5月3日 10:42",
        sortAt: "2026-05-03T10:42:00+08:00",
        isUnread: true,
        isStarred: true,
        hasAttachment: false,
        tags: ["系统通知"],
        isOutgoing: false,
        content: `
          <div class="border border-slate-200 rounded-lg p-6 bg-white max-w-2xl">
            <div class="flex items-center gap-3 mb-6">
              <div class="w-10 h-10 bg-[#E5F5FF] text-[#009BF5] rounded-full flex items-center justify-center font-bold text-lg">Y</div>
              <div>
                <h2 class="text-xl font-bold text-slate-800">商户入驻成功通知</h2>
                <p class="text-sm text-slate-500">悦享e食官方平台</p>
              </div>
            </div>
            <p class="text-slate-700 leading-relaxed mb-6">
              尊敬的合作伙伴，您好！您提交的商户入驻申请已审核通过。请前往控制台完善店铺信息、营业时段与配送配置，尽快开启线上经营。
            </p>
            <a href="#" class="inline-block bg-[#009BF5] text-white px-6 py-2 rounded-md font-medium">前往商家控制台</a>
            <p class="text-xs text-slate-400 mt-8 pt-4 border-t border-slate-100">此邮件由系统自动发送，请勿直接回复。</p>
          </div>
        `,
        attachments: [],
      },
      {
        id: "mail-2",
        folder: "inbox",
        previousFolder: "inbox",
        sender: "李设计",
        senderEmail: "li.design@yuexiang.com",
        recipients: ["myname@yuexiang.com"],
        avatar: "L",
        role: "内部员工",
        subject: "Q3 营销活动主视觉定稿（含附件）",
        snippet: "各位好，附件是本次Q3活动的最终定稿文件，请查收并确认...",
        time: "昨天",
        dateTimeLabel: "2026年5月2日 19:15",
        sortAt: "2026-05-02T19:15:00+08:00",
        isUnread: false,
        isStarred: false,
        hasAttachment: true,
        tags: ["附件"],
        isOutgoing: false,
        content: '<p class="text-slate-700">各位好，<br/><br/>附件是本次Q3活动的最终定稿文件，请查收并确认。如果没有问题，我们今天下午就发给开发团队进行页面制作。<br/><br/>谢谢！</p>',
        attachments: [
          { id: "file-1", name: "Q3主视觉定稿.pdf", sizeLabel: "2.4 MB", type: "pdf" },
        ],
      },
      {
        id: "mail-3",
        folder: "inbox",
        previousFolder: "inbox",
        sender: "张店长",
        senderEmail: "zhang.boss@external.com",
        recipients: ["myname@yuexiang.com"],
        avatar: "Z",
        role: "外部商户",
        subject: "关于上周配送延迟的投诉反馈",
        snippet: "上周五晚高峰期间，我们的多个订单出现了严重的配送延迟...",
        time: "5月1日",
        dateTimeLabel: "2026年5月1日 21:06",
        sortAt: "2026-05-01T21:06:00+08:00",
        isUnread: true,
        isStarred: false,
        hasAttachment: false,
        tags: ["投诉"],
        isOutgoing: false,
        content: '<p class="text-slate-700">你好，<br/><br/>上周五晚高峰期间，我们的多个订单出现了严重的配送延迟，导致客户退单率升高，希望平台能给出合理解释并优化调度策略。</p>',
        attachments: [],
      },
      {
        id: "mail-4",
        folder: "sent",
        previousFolder: "sent",
        sender: "MyName",
        senderEmail: "myname@yuexiang.com",
        recipients: ["merchant.partner@example.com"],
        avatar: "M",
        role: "悦享用户",
        subject: "诚邀入驻悦享e食",
        snippet: "您好，诚邀您加入悦享e食生态，附件中是平台招商政策及合作说明...",
        time: "4月29日",
        dateTimeLabel: "2026年4月29日 14:32",
        sortAt: "2026-04-29T14:32:00+08:00",
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["业务邀请"],
        isOutgoing: true,
        content: inviteCardHtml,
        attachments: [],
      },
      {
        id: "mail-5",
        folder: "sent",
        previousFolder: "sent",
        sender: "MyName",
        senderEmail: "myname@yuexiang.com",
        recipients: ["team@yuexiang.com"],
        avatar: "M",
        role: "悦享用户",
        subject: "本周运营复盘摘要",
        snippet: "这是本周商户增长、履约表现和活动投放的简要复盘，请查收...",
        time: "4月28日",
        dateTimeLabel: "2026年4月28日 18:20",
        sortAt: "2026-04-28T18:20:00+08:00",
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["工作邮件"],
        isOutgoing: true,
        content: '<p class="text-slate-700">团队同学好，<br/><br/>这是本周商户增长、履约表现和活动投放的简要复盘，请查收。详细版本我会在明早例会上补充说明。<br/><br/>谢谢。</p>',
        attachments: [],
      },
      {
        id: "mail-6",
        folder: "drafts",
        previousFolder: "drafts",
        sender: "MyName",
        senderEmail: "myname@yuexiang.com",
        recipients: ["beta.user@example.com"],
        avatar: "M",
        role: "悦享用户",
        subject: "悦享e食内测邀请说明",
        snippet: "您好，这里是本次内测的使用说明、反馈方式和奖励机制...",
        time: "4月27日",
        dateTimeLabel: "2026年4月27日 09:45",
        sortAt: "2026-04-27T09:45:00+08:00",
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["草稿"],
        isOutgoing: true,
        content: '<p class="text-slate-700">您好，<br/><br/>这里是本次内测的使用说明、反馈方式和奖励机制，待最终确认后发送。</p>',
        attachments: [],
      },
      {
        id: "mail-7",
        folder: "trash",
        previousFolder: "inbox",
        sender: "未知发件人",
        senderEmail: "promo@unknown-example.net",
        recipients: ["myname@yuexiang.com"],
        avatar: "U",
        role: "外部联系人",
        subject: "限时福利领取提醒",
        snippet: "您有一份待领取的限时福利，请尽快点击链接完成认证...",
        time: "4月23日",
        dateTimeLabel: "2026年4月23日 11:02",
        sortAt: "2026-04-23T11:02:00+08:00",
        isUnread: false,
        isStarred: false,
        hasAttachment: false,
        tags: ["垃圾邮件"],
        isOutgoing: false,
        content: '<p class="text-slate-700">这是一封垃圾邮件示例。</p>',
        attachments: [],
      },
      {
        id: "mail-8",
        folder: "inbox",
        previousFolder: "inbox",
        sender: "悦享安全中心",
        senderEmail: "security@yuexiang.com",
        recipients: ["myname@yuexiang.com"],
        avatar: "S",
        role: "平台官方",
        subject: "检测到新的网页登录",
        snippet: "我们检测到您的账号于今天 08:16 在新设备上登录，如非本人操作请立即修改密码...",
        time: "08:16",
        dateTimeLabel: "2026年5月3日 08:16",
        sortAt: "2026-05-03T08:16:00+08:00",
        isUnread: false,
        isStarred: true,
        hasAttachment: false,
        tags: ["安全提醒"],
        isOutgoing: false,
        content: '<p class="text-slate-700">我们检测到您的账号于今天 08:16 在新设备上登录，如非本人操作，请立即在安全中心修改密码并结束异常会话。</p>',
        attachments: [],
      },
    ],
  };
}
