# Seeds a disposable OpenProject database for the wiki API throwaway runtime
# check. Run with `bundle exec rails runner` inside the plugin image.
# Prints one JSON document with the ids and API tokens the HTTP cases need.
#
# Everything here is throwaway data. Nothing in this file may ever be run
# against a live OpenProject instance.

require "json"

admin = User.admin.active.where.not(type: "SystemUser").first or raise "no admin user after seeding"
User.current = admin

def ok!(call, what)
  raise "#{what} failed: #{call.errors.full_messages.join('; ')}" unless call.success?

  call.result
end

def role!(name, permissions)
  role = ProjectRole.find_or_initialize_by(name:)
  role.permissions = permissions
  role.save!
  role
end

def user!(login)
  user = User.find_or_initialize_by(login:)
  user.assign_attributes(firstname: login.capitalize,
                         lastname: "Throwaway",
                         mail: "#{login}@example.invalid",
                         status: Principal.statuses[:active],
                         admin: false)
  user.password = user.password_confirmation = "Throwaway-#{SecureRandom.hex(12)}!aA1"
  user.save!
  user
end

def token!(user)
  Token::API.where(user:).destroy_all
  token = Token::API.create!(user:, data: { name: "throwaway" })
  token.plain_value
end

def project!(admin, identifier, modules)
  existing = Project.find_by(identifier:)
  return existing if existing

  ok!(Projects::CreateService.new(user: admin)
        .call(name: identifier.tr("-", " ").capitalize,
              identifier:,
              public: false,
              workspace_type: "project",
              enabled_module_names: modules),
      "project #{identifier}")
end

def member!(admin, user, project, role)
  return if Member.exists?(principal: user, project:)

  ok!(Members::CreateService.new(user: admin)
        .call(principal: user, project:, roles: [role]),
      "membership #{user.login} in #{project.identifier}")
end

def page!(admin, project, title, text, **attrs)
  existing = project.wiki.pages.find_by(title:)
  return existing if existing

  ok!(WikiPages::CreateService.new(user: admin)
        .call(wiki: project.wiki, title:, text:, **attrs),
      "page #{title}")
end

editor_role = role!("Throwaway wiki editor", %i[view_wiki_pages edit_wiki_pages])
reader_role = role!("Throwaway wiki reader", %i[view_wiki_pages])
nowiki_role = role!("Throwaway no wiki", %i[view_work_packages])
manager_role = role!("Throwaway wiki manager", %i[view_wiki_pages edit_wiki_pages manage_wiki])

editor = user!("wikieditor")
reader = user!("wikireader")
nowiki = user!("wikinone")
outsider = user!("wikioutsider")
manager = user!("wikimanager")

main = project!(admin, "wiki-main", %w[wiki work_package_tracking])
other = project!(admin, "wiki-other", %w[wiki])
module_off = project!(admin, "wiki-module-off", %w[work_package_tracking])
no_record = project!(admin, "wiki-no-record", %w[wiki])

# A project whose wiki module is enabled but whose Wiki row is gone.
no_record.wiki&.destroy
no_record.reload

[main, other, module_off, no_record].each { |project| member!(admin, editor, project, editor_role) }
member!(admin, reader, main, reader_role)
member!(admin, nowiki, main, nowiki_role)
member!(admin, manager, main, manager_role)

start = page!(admin, main, "Start", "# Start\n\nSeeded start page.")
protected_page = page!(admin, main, "Protected", "Protected page.")
unless protected_page.protected?
  ok!(WikiPages::UpdateService.new(user: admin, model: protected_page).call(protected: true), "protect page")
end
foreign = page!(admin, other, "Foreign", "Page of another project.")

puts "SEED_JSON=" + JSON.generate(
  projects: {
    main: main.id,
    other: other.id,
    module_off: module_off.id,
    no_record: no_record.id
  },
  pages: {
    start: start.id,
    protected: protected_page.id,
    foreign: foreign.id
  },
  users: {
    editor: editor.id,
    manager: manager.id
  },
  tokens: {
    editor: token!(editor),
    reader: token!(reader),
    nowiki: token!(nowiki),
    outsider: token!(outsider),
    manager: token!(manager)
  },
  wiki_rows: Wiki.count
)
