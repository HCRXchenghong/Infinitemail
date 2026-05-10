import React from "react";
import { usePostOffice } from "../state/PostOfficeContext";
import { AppSidebar } from "../components/layout/AppSidebar";
import { Topbar } from "../components/layout/Topbar";
import { LoginView } from "../views/LoginView";
import { MailWorkspaceView } from "../components/mail/MailWorkspaceView";
import { ComposeView } from "../views/ComposeView";
import { ContactsView } from "../views/ContactsView";
import { SettingsView } from "../views/SettingsView";
import { POST_OFFICE_DEFAULT_FOLDER, POST_OFFICE_VISIBLE_MAIL_FOLDERS } from "../services/postOfficeContract";

const mailViews = POST_OFFICE_VISIBLE_MAIL_FOLDERS;

function SplashScreen() {
  return (
    <div className="min-h-screen bg-white flex items-center justify-center">
      <div className="text-center">
        <div className="text-[#009BF5] text-2xl font-bold">悦享邮局</div>
        <div className="text-sm text-slate-500 mt-2">正在载入工作台...</div>
      </div>
    </div>
  );
}

export default function PostOfficeApp() {
  const {
    isBootstrapping,
    isAuthenticated,
    currentView,
    folderCounts,
    profile,
    health,
    notice,
    roleSwitcherEnabled,
    actions,
  } = usePostOffice();

  if (isBootstrapping) {
    return <SplashScreen />;
  }

  if (!isAuthenticated) {
    return <LoginView />;
  }

  let content = null;

  if (mailViews.includes(currentView)) {
    content = <MailWorkspaceView folderId={currentView} />;
  } else if (currentView === "compose") {
    content = <ComposeView />;
  } else if (currentView === "contacts") {
    content = <ContactsView />;
  } else if (currentView === "settings") {
    content = <SettingsView />;
  } else {
    content = <MailWorkspaceView folderId={POST_OFFICE_DEFAULT_FOLDER} />;
  }

  return (
    <div className="h-screen w-full flex overflow-hidden bg-white font-sans text-slate-900 antialiased">
      <AppSidebar
        currentView={currentView}
        folderCounts={folderCounts}
        profile={profile}
        onNavigate={actions.setCurrentView}
        onLogout={actions.logout}
      />

      <div className="flex-1 flex flex-col min-w-0">
        <Topbar
          health={health}
          notice={notice}
          profile={profile}
          roleSwitcherEnabled={roleSwitcherEnabled}
          onSwitchRole={actions.switchRole}
          onDismissNotice={actions.dismissNotice}
        />
        <main className="flex-1 overflow-hidden relative">{content}</main>
      </div>

      <style
        dangerouslySetInnerHTML={{
          __html: `
            .toggle-checkbox:checked { transform: translateX(1.25rem); border-color: #009BF5; }
            .toggle-checkbox:checked + .toggle-label { background-color: #009BF5; }
            .toggle-checkbox { left: 0; z-index: 1; border-color: #cbd5e1; transition: all 0.3s; }
            .toggle-label { width: 2.5rem; background-color: #cbd5e1; transition: all 0.3s; }
          `,
        }}
      />
    </div>
  );
}
