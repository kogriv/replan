require "open_project/replan_overlay/overlay_loader"
require "open_project/replan_overlay/overlay_presenter"
require "open_project/replan_overlay/gantt_presenter"
require "open_project/replan_overlay/task_presenter"

module OpenProject
  module ReplanOverlay
    class OverlaysController < ApplicationController
      before_action :find_project
      before_action :authorize

      def show
        @state = OverlayLoader.new(project_identifier: @project.identifier).load
        @overlay = @state["overlay"]
        @work_packages = @state["work_packages"]
        @presenter = OverlayPresenter.new(overlay: @overlay, work_packages: @work_packages)
        @attention_rows = @presenter.attention_rows
      end

      def gantt
        @state = OverlayLoader.new(project_identifier: @project.identifier).load
        @overlay = @state["overlay"]
        @presenter = OverlayPresenter.new(overlay: @overlay, work_packages: @state["work_packages"])
        @gantt = GanttPresenter.new(overlay: @overlay, from: params[:from], to: params[:to], scale: params[:scale])
        @show_lanes = params[:lanes] != "0"
        @show_load = params[:load] != "0"
      end

      def work_package
        @state = OverlayLoader.new(project_identifier: @project.identifier).load
        @overlay = @state["overlay"]
        @work_packages = @state["work_packages"]
        @presenter = OverlayPresenter.new(overlay: @overlay, work_packages: @work_packages)
        @work_package_id = params[:work_package_id].to_s
        @work_package_row = @presenter.work_package_row(@work_package_id)
        @task = TaskPresenter.new(overlay: @overlay, row: @work_package_row) if @work_package_row
        # A work package the overlay does not know is not found, with the
        # explanation still on the page.
        render status: :not_found if @state["available"] && @work_package_row.nil?
      end

      private

      def find_project
        project_param = params[:project_id] || params[:id]
        @project = Project.find_by(identifier: project_param) || Project.find(project_param)
      end
    end
  end
end
