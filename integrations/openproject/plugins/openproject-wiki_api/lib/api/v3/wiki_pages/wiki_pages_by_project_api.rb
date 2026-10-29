# frozen_string_literal: true

module API
  module V3
    module WikiPages
      class WikiPagesByProjectAPI < ::API::OpenProjectAPI
        helpers ::API::Utilities::UrlPropsParsingHelper

        resources :wiki_pages do
          helpers do
            def wiki_available?
              @project.module_enabled?(:wiki) && @project.wiki.present?
            end
          end

          after_validation do
            fail ::API::Errors::NotFound.new unless wiki_available?
          end

          get do
            authorize_in_project(:view_wiki_pages, project: @project)

            pages = WikiPage
                      .visible(current_user)
                      .where(wiki_id: @project.wiki.id)
                      .order(:title)

            ::API::V3::WikiPages::WikiPageCollectionRepresenter.new(
              pages,
              self_link: api_v3_paths.wiki_pages_by_project(@project.id),
              current_user:,
              page: to_i_or_nil(params[:offset]),
              per_page: resolve_page_size(params[:pageSize])
            )
          end

          post &::API::V3::Utilities::Endpoints::Create.new(
            model: WikiPage,
            parse_representer: ::API::V3::WikiPages::WikiPagePayloadRepresenter,
            params_modifier: ->(params) { params.merge(wiki: @project.wiki) }
          ).mount
        end
      end
    end
  end
end
