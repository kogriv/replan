Rails.application.routes.draw do
  scope "projects/:project_id", as: "project" do
    get "replan_overlay",
        to: "open_project/replan_overlay/overlays#show",
        as: :replan_overlay
    get "replan_overlay/gantt",
        to: "open_project/replan_overlay/overlays#gantt",
        as: :replan_overlay_gantt
    get "replan_overlay/work_packages/:work_package_id",
        to: "open_project/replan_overlay/overlays#work_package",
        as: :replan_overlay_work_package
  end
end
