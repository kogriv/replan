module OpenProject
  module ReplanOverlay
    class Engine < ::Rails::Engine
      engine_name :openproject_replan_overlay

      include OpenProject::Plugins::ActsAsOpEngine

      initializer "#{engine_name}.add_routing_path", before: "add_routing_paths" do |app|
        app.config.paths["config/routes.rb"].unshift root.join("config", "routes.rb").to_s
      end

      register "openproject-replan_overlay",
               author_url: "https://github.com/kogriv/replan",
               requires_openproject: ">= 16.0.0" do
        project_module :replan_overlay do
          # Every controller action must be listed here: an action missing
          # from the permission is refused to everyone but admins.
          permission :view_replan_overlay,
                     { "open_project/replan_overlay/overlays" => %i[show gantt work_package] },
                     permissible_on: :project
        end

        menu :project_menu,
             :replan_overlay,
             { controller: "/open_project/replan_overlay/overlays", action: "show" },
             caption: "RePlan",
             icon: "graph-bar-vertical",
             after: :overview,
             param: :project_id

        menu :project_menu,
             :replan_overlay_gantt,
             { controller: "/open_project/replan_overlay/overlays", action: "gantt" },
             caption: "Gantt",
             parent: :replan_overlay,
             param: :project_id
      end
    end
  end
end
