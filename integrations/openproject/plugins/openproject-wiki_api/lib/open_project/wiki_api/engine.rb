# frozen_string_literal: true

module OpenProject
  module WikiAPI
    class Engine < ::Rails::Engine
      engine_name :openproject_wiki_api

      include OpenProject::Plugins::ActsAsOpEngine

      register "openproject-wiki_api",
               author_url: "https://github.com/kogriv/replan",
               requires_openproject: ">= 16.0.0"

      add_api_path :wiki_pages_by_project do |project_id|
        "#{project(project_id)}/wiki_pages"
      end

      add_api_endpoint "API::V3::Projects::ProjectsAPI", :id do
        mount ::API::V3::WikiPages::WikiPagesByProjectAPI
      end

      add_api_endpoint "API::V3::WikiPages::WikiPagesAPI", :id do
        after_validation do
          @wiki_page = WikiPage.visible(current_user).find(params[:id])
        end

        patch &::API::V3::Utilities::Endpoints::Update.new(
          model: WikiPage,
          parse_representer: ::API::V3::WikiPages::WikiPageUpdatePayloadRepresenter,
          params_modifier: ->(params) { params.reverse_merge(lock_version: nil) }
        ).mount

        delete &::API::V3::Utilities::Endpoints::Delete.new(
          model: WikiPage,
          process_service: ::API::V3::WikiPages::WikiPageDeleteService
        ).mount
      end

      extend_api_response(:v3, :wiki_pages, :wiki_page) do
        include ::API::Decorators::FormattableProperty
        include ::API::Decorators::DateProperty

        property :slug

        property :lock_version,
                 render_nil: true,
                 getter: ->(*) { lock_version.to_i }

        property :protected

        date_time_property :created_at
        date_time_property :updated_at

        formattable_property :text

        associated_resource :parent,
                            v3_path: :wiki_page,
                            representer: ::API::V3::WikiPages::WikiPageListItemRepresenter,
                            link_title_attribute: :title,
                            skip_link: ->(*) { represented.parent_id.nil? },
                            getter: ->(*) {}

        associated_resource :author,
                            v3_path: :user,
                            representer: ::API::V3::Users::UserRepresenter,
                            link_title_attribute: :name,
                            skip_link: ->(*) { represented.author_id.nil? },
                            getter: ->(*) {}

        link :update,
             cache_if: -> { current_user.allowed_in_project?(:edit_wiki_pages, represented.project) } do
          {
            href: api_v3_paths.wiki_page(represented.id),
            method: :patch
          }
        end

        link :delete,
             cache_if: -> { current_user.allowed_in_project?(:manage_wiki, represented.project) } do
          {
            href: api_v3_paths.wiki_page(represented.id),
            method: :delete
          }
        end
      end
    end
  end
end
